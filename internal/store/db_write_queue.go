package store

import "sync"

type WriteQueue struct {
	store *Store
	jobs  chan writeJob
	wg    sync.WaitGroup
}

type writeJob struct {
	typeName   string
	from       any
	run        func() error
	errorEvent map[string]any
}

func NewWriteQueue(s *Store) *WriteQueue {
	if s == nil {
		return nil
	}
	q := &WriteQueue{
		store: s,
		jobs:  make(chan writeJob, 1024),
	}
	q.wg.Add(1)
	go q.run()
	return q
}

func (q *WriteQueue) EnqueueRecord(record map[string]any, clientInfo MQTTClientInfo) {
	if q == nil {
		return
	}
	record = cloneDBWriteRecord(record)
	switch record["type"] {
	case "nodeinfo":
		q.enqueue(writeJob{typeName: "nodeinfo", from: record["from"], run: func() error {
			return q.store.UpsertNodeInfo(record)
		}})
	case "map_report":
		q.enqueue(writeJob{typeName: "map_report", from: record["from"], run: func() error {
			return q.store.UpsertMapReport(record)
		}})
	case "text_message":
		// 私聊（PKI 加密、发往受管 bot）单独走 bot_direct_messages 表，
		// 不再写入 text_message 以避免和频道消息混在一起。
		if isInboundBotDirectMessage(q.store, record) {
			q.enqueue(writeJob{typeName: "bot_direct_message_inbound", from: record["from"], run: func() error {
				return insertInboundBotDirectMessage(q.store, record, clientInfo)
			}})
			return
		}
		// 频道消息同时也写入 LLM 队列（如果启用的话）
		q.enqueue(writeJob{typeName: "llm_channel_message", from: record["from"], run: func() error {
			return enqueueChannelMessageToLLM(q.store, record)
		}})
		q.enqueue(writeJob{typeName: "text_message", from: record["from"], run: func() error {
			return q.store.InsertTextMessage(record, clientInfo)
		}})
	case "position":
		q.enqueue(writeJob{typeName: "position", from: record["from"], run: func() error {
			return q.store.InsertPosition(record, clientInfo)
		}})
	case "telemetry":
		q.enqueue(writeJob{typeName: "telemetry", from: record["from"], run: func() error {
			return q.store.InsertTelemetry(record, clientInfo)
		}})
	case "routing":
		q.enqueue(writeJob{typeName: "routing", from: record["from"], run: func() error {
			return q.store.InsertRouting(record, clientInfo)
		}})
	case "traceroute":
		q.enqueue(writeJob{typeName: "traceroute", from: record["from"], run: func() error {
			return q.store.InsertTraceroute(record, clientInfo)
		}})
	}
}

func (q *WriteQueue) EnqueueDiscard(record map[string]any, raw []byte, clientInfo MQTTClientInfo) {
	if q == nil {
		return
	}
	record = cloneDBWriteRecord(record)
	raw = append([]byte(nil), raw...)
	q.enqueue(writeJob{typeName: "discard_details", from: record["from"], errorEvent: map[string]any{"event": "db_error", "type": "discard_details", "topic": record["topic"]}, run: func() error {
		return q.store.InsertDiscardDetails(record, raw, clientInfo)
	}})
}

func (q *WriteQueue) Close() {
	if q == nil {
		return
	}
	close(q.jobs)
	q.wg.Wait()
}

func (q *WriteQueue) Len() int {
	if q == nil {
		return 0
	}
	return len(q.jobs)
}

func (q *WriteQueue) enqueue(job writeJob) {
	q.jobs <- job
}

func (q *WriteQueue) run() {
	defer q.wg.Done()
	for job := range q.jobs {
		if err := job.run(); err != nil {
			event := job.errorEvent
			if event == nil {
				event = map[string]any{"event": "db_error", "type": job.typeName, "from": job.from}
			} else {
				event = cloneDBWriteRecord(event)
			}
			event["error"] = err.Error()
			printJSON(event)
		}
	}
}

func cloneDBWriteRecord(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	cloned := make(map[string]any, len(record))
	for key, value := range record {
		cloned[key] = value
	}
	return cloned
}
