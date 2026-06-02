#!/usr/bin/env python3
"""Subscribe to a Meshtastic MQTT broker and print public/decoded node info.

This helper is intended for MQTT brokers and channels you are authorized to
monitor. Encrypted mesh packets are decrypted when they match the configured
channel PSK; packets that cannot be decrypted are reported as metadata.

Dependencies:
    pip install paho-mqtt meshtastic protobuf cryptography

Example:
    python pytest/mqtt_nodeinfo_subscriber.py
    python pytest/mqtt_nodeinfo_subscriber.py --topic 'msh/US/#'
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
from typing import Any

import paho.mqtt.client as mqtt
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from google.protobuf.message import DecodeError
from meshtastic.protobuf import mesh_pb2, mqtt_pb2, portnums_pb2


DEFAULT_HOST = "mqtt.meshtastic.org"
DEFAULT_USERNAME = "meshdev"
DEFAULT_PASSWORD = "large4cats"
DEFAULT_PSK = "AQ=="
DEFAULT_TOPICS = ("msh/US/#",)
ANSI_GREEN_BG_WHITE_TEXT = "\033[42;37m"
ANSI_RESET = "\033[0m"
DEFAULT_MESHTASTIC_PSK = bytes(
    [0xD4, 0xF1, 0xBB, 0x3A, 0x20, 0x29, 0x07, 0x59, 0xF0, 0xBC, 0xFF, 0xAB, 0xCF, 0x4E, 0x69, 0x01]
)


def node_num_to_id(node_num: int) -> str:
    return f"!{node_num:08x}"


def enum_name(enum_type: Any, value: int) -> str | int:
    try:
        return enum_type.Name(value)
    except ValueError:
        return value


def xor_hash(data: bytes) -> int:
    result = 0
    for byte in data:
        result ^= byte
    return result


def expand_psk(psk_base64: str) -> bytes:
    psk = base64.b64decode(psk_base64)
    if len(psk) == 1:
        psk_index = psk[0]
        if psk_index == 0:
            return b""
        key = bytearray(DEFAULT_MESHTASTIC_PSK)
        key[-1] = (key[-1] + psk_index - 1) & 0xFF
        return bytes(key)
    if 0 < len(psk) < 16:
        return psk.ljust(16, b"\x00")
    if 16 < len(psk) < 32:
        return psk.ljust(32, b"\x00")
    return psk


def channel_hash(channel_name: str, key: bytes) -> int:
    return xor_hash(channel_name.encode()) ^ xor_hash(key)


def decrypt_aes_ctr(key: bytes, from_num: int, packet_id: int, ciphertext: bytes) -> bytes:
    nonce = bytearray(16)
    nonce[0:8] = packet_id.to_bytes(8, "little")
    nonce[8:12] = from_num.to_bytes(4, "little")
    cipher = Cipher(algorithms.AES(key), modes.CTR(bytes(nonce)))
    decryptor = cipher.decryptor()
    return decryptor.update(ciphertext) + decryptor.finalize()


def try_decrypt_packet(packet: mesh_pb2.MeshPacket, channel_id: str, key: bytes) -> tuple[mesh_pb2.MeshPacket | None, str]:
    if not key:
        return None, "psk disables encryption"
    if packet.channel != channel_hash(channel_id, key):
        return None, "channel hash mismatch"

    plaintext = decrypt_aes_ctr(key, mesh_packet_from_field(packet), packet.id, packet.encrypted)
    decoded = mesh_pb2.Data()
    try:
        decoded.ParseFromString(plaintext)
    except DecodeError as exc:
        return None, f"decrypted bytes are not Data protobuf: {exc}"
    if decoded.portnum == portnums_pb2.UNKNOWN_APP:
        return None, "decrypted protobuf has UNKNOWN_APP portnum"

    decrypted_packet = mesh_pb2.MeshPacket()
    decrypted_packet.CopyFrom(packet)
    decrypted_packet.ClearField("encrypted")
    decrypted_packet.decoded.CopyFrom(decoded)
    return decrypted_packet, "success"


def mesh_packet_from_field(packet: mesh_pb2.MeshPacket) -> int:
    # The protobuf field is named "from" in proto, but generated Python exposes
    # it as "from" via getattr because "from" is a Python keyword.
    return getattr(packet, "from")


def decode_user(packet: mesh_pb2.MeshPacket) -> dict[str, Any]:
    user = mesh_pb2.User()
    user.ParseFromString(packet.decoded.payload)

    return {
        "type": "nodeinfo",
        "from": node_num_to_id(mesh_packet_from_field(packet)),
        "from_num": mesh_packet_from_field(packet),
        "user_id": user.id,
        "long_name": user.long_name,
        "short_name": user.short_name,
        "hw_model": enum_name(mesh_pb2.HardwareModel, user.hw_model),
        "role": enum_name(mesh_pb2.Config.DeviceConfig.Role, user.role),
        "is_licensed": user.is_licensed,
        "public_key": user.public_key.hex() if user.public_key else None,
    }


def decode_map_report(packet: mesh_pb2.MeshPacket) -> dict[str, Any]:
    report = mqtt_pb2.MapReport()
    report.ParseFromString(packet.decoded.payload)

    return {
        "type": "map_report",
        "from": node_num_to_id(mesh_packet_from_field(packet)),
        "from_num": mesh_packet_from_field(packet),
        "long_name": report.long_name,
        "short_name": report.short_name,
        "role": enum_name(mesh_pb2.Config.DeviceConfig.Role, report.role),
        "hw_model": enum_name(mesh_pb2.HardwareModel, report.hw_model),
        "firmware_version": report.firmware_version,
        "region": enum_name(mesh_pb2.Config.LoRaConfig.RegionCode, report.region),
        "modem_preset": enum_name(mesh_pb2.Config.LoRaConfig.ModemPreset, report.modem_preset),
        "latitude": report.latitude_i * 1e-7 if report.latitude_i else None,
        "longitude": report.longitude_i * 1e-7 if report.longitude_i else None,
        "altitude": report.altitude,
        "position_precision": report.position_precision,
        "num_online_local_nodes": report.num_online_local_nodes,
        "has_opted_report_location": report.has_opted_report_location,
    }


def describe_packet(topic: str, env: mqtt_pb2.ServiceEnvelope, key: bytes) -> dict[str, Any]:
    packet = env.packet
    from_num = mesh_packet_from_field(packet)
    payload_variant = packet.WhichOneof("payload_variant")

    base = {
        "topic": topic,
        "channel_id": env.channel_id,
        "gateway_id": env.gateway_id,
        "packet_from": node_num_to_id(from_num),
        "packet_from_num": from_num,
        "packet_to": node_num_to_id(packet.to),
        "packet_to_num": packet.to,
        "packet_id": packet.id,
        "payload_variant": payload_variant,
        "via_mqtt": packet.via_mqtt,
        "pki_encrypted": packet.pki_encrypted,
    }

    if payload_variant == "encrypted":
        decrypted_packet, decrypt_status = try_decrypt_packet(packet, env.channel_id, key)
        if decrypted_packet is None:
            return {
                **base,
                "type": "encrypted_packet",
                "encrypted_len": len(packet.encrypted),
                "decrypt_success": False,
                "decrypt_status": decrypt_status,
            }

        decrypted_env = mqtt_pb2.ServiceEnvelope()
        decrypted_env.CopyFrom(env)
        decrypted_env.packet.CopyFrom(decrypted_packet)
        decrypted = describe_packet(topic, decrypted_env, key)
        decrypted["payload_variant"] = "decoded"
        decrypted["decrypt_success"] = True
        decrypted["decrypt_status"] = decrypt_status
        return decrypted

    if payload_variant != "decoded":
        return {**base, "type": "empty_packet"}

    portnum = packet.decoded.portnum
    decoded_base = {
        **base,
        "portnum": enum_name(portnums_pb2.PortNum, portnum),
        "payload_len": len(packet.decoded.payload),
    }

    if portnum == portnums_pb2.NODEINFO_APP:
        return {**decoded_base, **decode_user(packet)}

    if portnum == portnums_pb2.MAP_REPORT_APP:
        return {**decoded_base, **decode_map_report(packet)}

    return {**decoded_base, "type": "decoded_packet"}


def print_json(record: dict[str, Any]) -> None:
    text = json.dumps(record, ensure_ascii=False, sort_keys=True)
    if record.get("decrypt_success") is True:
        text = f"{ANSI_GREEN_BG_WHITE_TEXT}{text}{ANSI_RESET}"
    print(text, flush=True)


def on_connect(client: mqtt.Client, userdata: argparse.Namespace, flags: Any, reason_code: Any, properties: Any = None) -> None:
    print_json({"event": "connected", "reason_code": str(reason_code)})
    for topic in userdata.topics:
        client.subscribe(topic, qos=userdata.qos)
        print_json({"event": "subscribed", "topic": topic, "qos": userdata.qos})


def on_message(client: mqtt.Client, userdata: argparse.Namespace, msg: mqtt.MQTTMessage) -> None:
    try:
        env = mqtt_pb2.ServiceEnvelope()
        env.ParseFromString(msg.payload)
        print_json(describe_packet(msg.topic, env, userdata.key))
    except DecodeError as exc:
        print_json({"topic": msg.topic, "error": f"protobuf decode failed: {exc}", "payload_len": len(msg.payload)})
    except Exception as exc:  # Keep the subscriber alive while reporting malformed packets.
        print_json({"topic": msg.topic, "error": str(exc), "payload_len": len(msg.payload)})


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Subscribe to Meshtastic MQTT and print decoded public node info as JSONL.")
    parser.add_argument("--host", default=DEFAULT_HOST, help="MQTT broker hostname")
    parser.add_argument("--port", type=int, default=1883, help="MQTT broker port")
    parser.add_argument("--username", default=DEFAULT_USERNAME, help="MQTT username")
    parser.add_argument("--password", default=DEFAULT_PASSWORD, help="MQTT password")
    parser.add_argument("--psk", default=DEFAULT_PSK, help="Base64 channel PSK used to try decrypting encrypted packets")
    parser.add_argument(
        "--topic",
        action="append",
        dest="topics",
        help="Topic to subscribe; may be repeated. Defaults to msh/US/#",
    )
    parser.add_argument("--qos", type=int, default=0, choices=(0, 1, 2), help="MQTT subscription QoS")
    parser.add_argument("--client-id", default="meshtastic-nodeinfo-subscriber", help="MQTT client id")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.topics:
        args.topics = list(DEFAULT_TOPICS)
    args.key = expand_psk(args.psk)

    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=args.client_id)
    client.user_data_set(args)
    client.on_connect = on_connect
    client.on_message = on_message

    if args.username is not None:
        client.username_pw_set(args.username, args.password)

    client.connect(args.host, args.port, keepalive=60)
    client.loop_forever()
    return 0


if __name__ == "__main__":
    sys.exit(main())
