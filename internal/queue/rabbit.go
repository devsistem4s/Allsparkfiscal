package queue

import (
	"allspark.local/fiscal/internal/store"
	"context"
	"encoding/json"
	"errors"
	"github.com/rabbitmq/amqp091-go"
	"time"
)

const Main = "fiscal.jobs"
const Dead = "fiscal.dead"

type Broker struct {
	Conn    *amqp091.Connection
	Channel *amqp091.Channel
	Returns <-chan amqp091.Return
}

func Open(url string) (*Broker, error) {
	c, e := amqp091.DialConfig(url, amqp091.Config{Heartbeat: 10 * time.Second, Dial: amqp091.DefaultDial(10 * time.Second)})
	if e != nil {
		return nil, e
	}
	ch, e := c.Channel()
	if e != nil {
		c.Close()
		return nil, e
	}
	b := &Broker{Conn: c, Channel: ch, Returns: ch.NotifyReturn(make(chan amqp091.Return, 1))}
	for _, name := range []string{Dead, Main} {
		args := amqp091.Table{"x-queue-type": "quorum"}
		if name == Main {
			args["x-dead-letter-exchange"] = ""
			args["x-dead-letter-routing-key"] = Dead
		}
		if _, e = ch.QueueDeclare(name, true, false, false, false, args); e != nil {
			b.Close()
			return nil, e
		}
	}
	return b, nil
}
func (b *Broker) Close() { _ = b.Channel.Close(); _ = b.Conn.Close() }

type Message struct {
	JobID string `json:"job_id"`
}

func (b *Broker) Dispatch(ctx context.Context, s *store.Store) error {
	if e := b.Channel.Confirm(false); e != nil {
		return e
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			for n := 0; n < 20; n++ {
				sent, e := b.publishBatch(ctx, s, 100)
				if e != nil {
					return e
				}
				if sent == 0 {
					break
				}
			}
		}
	}
}

type outboxItem struct {
	id, route string
}

func (b *Broker) publishBatch(ctx context.Context, s *store.Store, limit int) (int, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, "SELECT job_id::text,routing_key FROM outbox WHERE published_at IS NULL AND available_at<=now() ORDER BY available_at FOR UPDATE SKIP LOCKED LIMIT $1", limit)
	if e != nil {
		return 0, e
	}
	var items []outboxItem
	for rows.Next() {
		var it outboxItem
		if e = rows.Scan(&it.id, &it.route); e != nil {
			rows.Close()
			return 0, e
		}
		items = append(items, it)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return 0, e
	}
	rows.Close()
	if len(items) == 0 {
		return 0, nil
	}
	pubctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	confirms := make([]*amqp091.DeferredConfirmation, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, it := range items {
		body, _ := json.Marshal(Message{it.id})
		confirm, e := b.Channel.PublishWithDeferredConfirmWithContext(pubctx, "", it.route, true, false, amqp091.Publishing{
			DeliveryMode: amqp091.Persistent,
			ContentType:  "application/json",
			MessageId:    it.id,
			Body:         body,
		})
		if e != nil {
			return 0, e
		}
		if confirm == nil {
			return 0, errors.New("publisher confirmation missing")
		}
		confirms = append(confirms, confirm)
		ids = append(ids, it.id)
	}
	for _, c := range confirms {
		ok, e := c.WaitContext(pubctx)
		if e != nil {
			return 0, e
		}
		if !ok {
			return 0, errors.New("broker rejected publication")
		}
	}
	select {
	case <-b.Returns:
		return 0, errors.New("unroutable publication")
	default:
	}
	if _, e = tx.Exec(ctx, "UPDATE outbox SET published_at=now() WHERE job_id=ANY($1::uuid[])", ids); e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return len(items), nil
}
