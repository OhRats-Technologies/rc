package transport

import (
	"testing"
	"time"
)

func TestDataChannelRoundTrip(t *testing.T) {
	offerer, err := NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer offerer.Close()
	answerer, err := NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer answerer.Close()

	offer, err := offerer.Offer()
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	answer, err := answerer.AcceptOffer(offer)
	if err != nil {
		t.Fatalf("accept offer: %v", err)
	}
	if err := offerer.AcceptAnswer(answer); err != nil {
		t.Fatalf("accept answer: %v", err)
	}
	if err := offerer.WaitOpen(10 * time.Second); err != nil {
		t.Fatalf("offerer open: %v", err)
	}
	if err := answerer.WaitOpen(10 * time.Second); err != nil {
		t.Fatalf("answerer open: %v", err)
	}
	if err := offerer.Send([]byte("ping")); err != nil {
		t.Fatalf("send: %v", err)
	}
	message, err := answerer.Receive(5 * time.Second)
	if err != nil || string(message) != "ping" {
		t.Fatalf("receive = %q, %v", message, err)
	}
	if err := answerer.Send([]byte("pong")); err != nil {
		t.Fatalf("send reply: %v", err)
	}
	message, err = offerer.Receive(5 * time.Second)
	if err != nil || string(message) != "pong" {
		t.Fatalf("receive reply = %q, %v", message, err)
	}
}
