package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func (s *store) InsertDiscardDetails(record map[string]any, raw []byte, clientInfo mqttClientInfo) error {
	details, err := discardDetailsFromRecord(record, raw, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(details).Error; err != nil {
		return fmt.Errorf("insert discard_details: %w", err)
	}
	return nil
}

func discardDetailsFromRecord(record map[string]any, raw []byte, clientInfo mqttClientInfo) (*discardDetailsRecord, error) {
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode discard_details content_json: %w", err)
	}
	return &discardDetailsRecord{
		Topic:          stringValue(record["topic"]),
		Error:          stringValue(record["error"]),
		PayloadLen:     int64(len(raw)),
		RawBase64:      base64.StdEncoding.EncodeToString(raw),
		ContentJSON:    string(contentJSON),
		MQTTClientID:   nullableStringValue(clientInfo.ClientID),
		MQTTUsername:   nullableStringValue(clientInfo.Username),
		MQTTListener:   nullableStringValue(clientInfo.Listener),
		MQTTRemoteAddr: nullableStringValue(clientInfo.RemoteAddr),
		MQTTRemoteHost: nullableStringValue(clientInfo.RemoteHost),
		MQTTRemotePort: nullableStringValue(clientInfo.RemotePort),
	}, nil
}

func stringValue(value any) string {
	if s := nullableStringValue(value); s != nil {
		return *s
	}
	return ""
}
