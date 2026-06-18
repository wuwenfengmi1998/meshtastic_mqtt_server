package mqttforward

import "sync/atomic"

type Stats struct {
	forwarded atomic.Int64
	dropped   atomic.Int64
}

func (s *Stats) IncForwarded() {
	if s != nil {
		s.forwarded.Add(1)
	}
}

func (s *Stats) IncDropped() {
	if s != nil {
		s.dropped.Add(1)
	}
}

func (s *Stats) Forwarded() int64 {
	if s == nil {
		return 0
	}
	return s.forwarded.Load()
}

func (s *Stats) Dropped() int64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}
