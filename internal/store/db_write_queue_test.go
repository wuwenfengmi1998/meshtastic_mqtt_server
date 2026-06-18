package store

import (
	"database/sql"
	"testing"
)

func TestDBWriteQueueWritesRecordsAsync(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	queue := newDBWriteQueue(st)
	record := textMessageTestRecord("queued")
	queue.EnqueueRecord(record, MQTTClientInfo{ClientID: "client-1"})
	record["text"] = "mutated after enqueue"
	queue.Close()

	var text, clientID string
	if err := rawTestDB(t, st).QueryRow("SELECT text, mqtt_client_id FROM text_message WHERE from_id = ?", "!12345678").Scan(&text, &clientID); err != nil {
		t.Fatal(err)
	}
	if text != "queued" || clientID != "client-1" {
		t.Fatalf("queued row = text %q client %q, want queued/client-1", text, clientID)
	}
}

func TestDBWriteQueueWritesDiscardAsync(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	queue := newDBWriteQueue(st)
	record := map[string]any{"topic": "msh/test", "error": "bad packet"}
	queue.EnqueueDiscard(record, []byte{1, 2, 3}, MQTTClientInfo{RemoteAddr: "127.0.0.1:1883"})
	record["error"] = "mutated after enqueue"
	queue.Close()

	var topic, reason, rawBase64, remoteAddr string
	if err := rawTestDB(t, st).QueryRow("SELECT topic, error, raw_base64, mqtt_remote_addr FROM discard_details").Scan(&topic, &reason, &rawBase64, &remoteAddr); err != nil {
		t.Fatal(err)
	}
	if topic != "msh/test" || reason != "bad packet" || rawBase64 != "AQID" || remoteAddr != "127.0.0.1:1883" {
		t.Fatalf("discard row = %q/%q/%q/%q, want queued values", topic, reason, rawBase64, remoteAddr)
	}
}

func TestDBWriteQueueLen(t *testing.T) {
	queue := &WriteQueue{jobs: make(chan writeJob, 1)}
	queue.enqueue(writeJob{run: func() error { return nil }})
	if queue.Len() != 1 {
		t.Fatalf("queue.Len() = %d, want 1", queue.Len())
	}
}

func TestDBWriteQueueIgnoresUnsupportedRecordType(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	queue := newDBWriteQueue(st)
	queue.EnqueueRecord(map[string]any{"type": "empty_packet", "from": "!12345678"}, MQTTClientInfo{})
	queue.Close()

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM text_message").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("text_message count = %d, want 0", count)
	}
}

func TestDBWriteQueueNilStore(t *testing.T) {
	if queue := newDBWriteQueue(nil); queue != nil {
		t.Fatalf("newDBWriteQueue(nil) = %#v, want nil", queue)
	}
	var queue *WriteQueue
	queue.EnqueueRecord(textMessageTestRecord("ignored"), MQTTClientInfo{})
	queue.EnqueueDiscard(map[string]any{"topic": "ignored"}, []byte{1}, MQTTClientInfo{})
	queue.Close()
}

func TestDBWriteQueueRecordValidationErrorDoesNotStopWorker(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	queue := newDBWriteQueue(st)
	badRecord := textMessageTestRecord("bad")
	delete(badRecord, "from")
	queue.EnqueueRecord(badRecord, MQTTClientInfo{})
	queue.EnqueueRecord(textMessageTestRecord("good"), MQTTClientInfo{})
	queue.Close()

	var text string
	if err := rawTestDB(t, st).QueryRow("SELECT text FROM text_message").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "good" {
		t.Fatalf("text = %q, want good", text)
	}

	var missing sql.NullString
	if err := rawTestDB(t, st).QueryRow("SELECT text FROM text_message WHERE text = ?", "bad").Scan(&missing); err != sql.ErrNoRows {
		t.Fatalf("bad row error = %v, want sql.ErrNoRows", err)
	}
}
