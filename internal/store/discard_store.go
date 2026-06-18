package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func (s *Store) InsertDiscardDetails(record map[string]any, raw []byte, clientInfo MQTTClientInfo) error {
	details, err := discardDetailsFromRecord(record, raw, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(details).Error; err != nil {
		return fmt.Errorf("insert discard_details: %w", err)
	}
	return nil
}

func discardDetailsFromRecord(record map[string]any, raw []byte, clientInfo MQTTClientInfo) (*DiscardDetailsRecord, error) {
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode discard_details content_json: %w", err)
	}
	return &DiscardDetailsRecord{
		Topic:          stringValue(record["topic"]),
		Error:          stringValue(record["error"]),
		PayloadLen:     int64(len(raw)),
		RawBase64:      base64.StdEncoding.EncodeToString(raw),
		ContentJSON:    string(contentJSON),
		MQTTClientID:   NullableStringValue(clientInfo.ClientID),
		MQTTUsername:   NullableStringValue(clientInfo.Username),
		MQTTListener:   NullableStringValue(clientInfo.Listener),
		MQTTRemoteAddr: NullableStringValue(clientInfo.RemoteAddr),
		MQTTRemoteHost: NullableStringValue(clientInfo.RemoteHost),
		MQTTRemotePort: NullableStringValue(clientInfo.RemotePort),
	}, nil
}

func stringValue(value any) string {
	if s := NullableStringValue(value); s != nil {
		return *s
	}
	return ""
}
