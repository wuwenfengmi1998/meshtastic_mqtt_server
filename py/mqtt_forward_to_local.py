#!/usr/bin/env python3
"""Forward Meshtastic MQTT publishes from the public broker to a local broker.

Dependencies:
    pip install paho-mqtt

Example:
    python py/mqtt_forward_to_local.py
    python py/mqtt_forward_to_local.py --local-host 127.0.0.1 --local-port 1883
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any

import paho.mqtt.client as mqtt


DEFAULT_HOST = "mqtt.mess.host"
DEFAULT_USERNAME = "meshdev"
DEFAULT_PASSWORD = "large4cats"
DEFAULT_TOPICS = ("msh/#",)
SOURCE_TOPIC_PREFIX = "msh/CN"
LOCAL_TOPIC_PREFIX = "msh/CN"
DEFAULT_LOCAL_HOST = "mesh.lmve.net"
DEFAULT_LOCAL_PORT = 1883


def print_json(record: dict[str, Any]) -> None:
    print(json.dumps(record, ensure_ascii=False, separators=(",", ":")), flush=True)


def on_local_connect(client: mqtt.Client, userdata: argparse.Namespace, flags: Any, reason_code: Any, properties: Any = None) -> None:
    print_json({"event": "local_connected", "host": userdata.local_host, "port": userdata.local_port, "reason_code": str(reason_code)})


def on_source_connect(client: mqtt.Client, userdata: argparse.Namespace, flags: Any, reason_code: Any, properties: Any = None) -> None:
    print_json({"event": "source_connected", "host": userdata.host, "port": userdata.port, "reason_code": str(reason_code)})
    for topic in userdata.topics:
        client.subscribe(topic, qos=userdata.qos)
        print_json({"event": "source_subscribed", "topic": topic, "qos": userdata.qos})


def local_topic(source_topic: str) -> str:
    if source_topic == SOURCE_TOPIC_PREFIX or source_topic.startswith(SOURCE_TOPIC_PREFIX + "/"):
        return LOCAL_TOPIC_PREFIX + source_topic[len(SOURCE_TOPIC_PREFIX) :]
    return source_topic


def on_source_message(client: mqtt.Client, userdata: argparse.Namespace, msg: mqtt.MQTTMessage) -> None:
    topic = local_topic(msg.topic)
    result = userdata.local_client.publish(topic, payload=msg.payload, qos=msg.qos, retain=msg.retain)
    print_json(
        {
            "event": "forwarded",
            "source_topic": msg.topic,
            "topic": topic,
            "payload_len": len(msg.payload),
            "qos": msg.qos,
            "retain": msg.retain,
            "result": result.rc,
        }
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Forward Meshtastic MQTT messages from mqtt.meshtastic.org to a local MQTT broker.")
    parser.add_argument("--host", default=DEFAULT_HOST, help="Source MQTT broker hostname")
    parser.add_argument("--port", type=int, default=1883, help="Source MQTT broker port")
    parser.add_argument("--username", default=DEFAULT_USERNAME, help="Source MQTT username")
    parser.add_argument("--password", default=DEFAULT_PASSWORD, help="Source MQTT password")
    parser.add_argument(
        "--topic",
        action="append",
        dest="topics",
        help="Source topic to subscribe; may be repeated. Defaults to msh/US/#",
    )
    parser.add_argument("--qos", type=int, default=0, choices=(0, 1, 2), help="Source subscription QoS")
    parser.add_argument("--client-id", default="meshtastic-forward-source", help="Source MQTT client id")
    parser.add_argument("--local-host", default=DEFAULT_LOCAL_HOST, help="Local MQTT broker hostname")
    parser.add_argument("--local-port", type=int, default=DEFAULT_LOCAL_PORT, help="Local MQTT broker port")
    parser.add_argument("--local-client-id", default="meshtastic-forward-local", help="Local MQTT client id")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.topics:
        args.topics = list(DEFAULT_TOPICS)

    local_client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=args.local_client_id)
    local_client.user_data_set(args)
    local_client.on_connect = on_local_connect
    local_client.connect(args.local_host, args.local_port, keepalive=60)
    local_client.loop_start()
    args.local_client = local_client

    source_client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=args.client_id)
    source_client.user_data_set(args)
    source_client.on_connect = on_source_connect
    source_client.on_message = on_source_message
    if args.username is not None:
        source_client.username_pw_set(args.username, args.password)

    source_client.connect(args.host, args.port, keepalive=60)
    try:
        source_client.loop_forever()
    finally:
        local_client.loop_stop()
        local_client.disconnect()
    return 0


if __name__ == "__main__":
    sys.exit(main())
