package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/lib/pq"
)

// Scope travels only on the private database channel, never to browser clients.
type liveEnvelope struct {
	Message BroadcastMessage
	Scope   socialScope
}

var publishLive func(BroadcastMessage)

func (d *Database) startLiveUpdates(dsn string) (func(), error) {
	listener := pq.NewListener(dsn, time.Second, 30*time.Second, func(_ pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Live update listener: %v", err)
		}
	})
	if err := listener.Listen("bible_updates"); err != nil {
		listener.Close()
		return nil, err
	}
	publishLive = func(msg BroadcastMessage) {
		data, err := json.Marshal(liveEnvelope{msg, msg.Scope})
		if err == nil {
			_, err = d.db.Exec("SELECT pg_notify('bible_updates', ?)", string(data))
		}
		if err != nil {
			log.Printf("Could not publish live update: %v", err)
			queueLiveUpdate(msg)
		}
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case n := <-listener.Notify:
				if n == nil {
					continue
				}
				var event liveEnvelope
				if json.Unmarshal([]byte(n.Extra), &event) == nil {
					event.Message.Scope = event.Scope
					queueLiveUpdate(event.Message)
				}
			}
		}
	}()
	return func() { close(done); listener.Close() }, nil
}
