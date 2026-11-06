// Package transport provides the authenticated WebRTC DataChannel transport
// used for browser/CLI control sessions.
package transport

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// Peer is one WebRTC endpoint with the RC control DataChannel.
type Peer struct {
	mu       sync.Mutex
	pc       *webrtc.PeerConnection
	channel  *webrtc.DataChannel
	opened   chan struct{}
	messages chan []byte
	closed   chan struct{}
	once     sync.Once
}

// NewPeer creates a peer with the given ICE servers.
func NewPeer(iceServers []webrtc.ICEServer) (*Peer, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
	if err != nil {
		return nil, err
	}
	peer := &Peer{
		pc:       pc,
		opened:   make(chan struct{}),
		messages: make(chan []byte, 64),
		closed:   make(chan struct{}),
	}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			peer.Close()
		}
	})
	return peer, nil
}

// Offer creates the control DataChannel and returns a non-trickle offer.
func (p *Peer) Offer() (string, error) {
	channel, err := p.pc.CreateDataChannel("rc-control", nil)
	if err != nil {
		return "", err
	}
	p.attach(channel)
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}
	if err := p.pc.SetLocalDescription(offer); err != nil {
		return "", err
	}
	if err := p.gather(); err != nil {
		return "", err
	}
	return p.pc.LocalDescription().SDP, nil
}

// AcceptOffer applies a remote offer and returns a non-trickle answer.
func (p *Peer) AcceptOffer(sdp string) (string, error) {
	p.pc.OnDataChannel(func(channel *webrtc.DataChannel) { p.attach(channel) })
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return "", err
	}
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	if err := p.gather(); err != nil {
		return "", err
	}
	return p.pc.LocalDescription().SDP, nil
}

// AcceptAnswer applies the remote answer.
func (p *Peer) AcceptAnswer(sdp string) error {
	return p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
}

func (p *Peer) attach(channel *webrtc.DataChannel) {
	p.mu.Lock()
	p.channel = channel
	p.mu.Unlock()
	channel.OnOpen(func() {
		p.once.Do(func() { close(p.opened) })
	})
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		select {
		case p.messages <- append([]byte(nil), message.Data...):
		case <-p.closed:
		}
	})
}

func (p *Peer) gather() error {
	complete := make(chan struct{})
	p.pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		if state == webrtc.ICEGatheringStateComplete {
			select {
			case <-complete:
			default:
				close(complete)
			}
		}
	})
	select {
	case <-complete:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("ICE gathering timed out")
	}
}

// WaitOpen waits for the DataChannel to open.
func (p *Peer) WaitOpen(timeout time.Duration) error {
	select {
	case <-p.opened:
		return nil
	case <-p.closed:
		return errors.New("peer connection closed")
	case <-time.After(timeout):
		return errors.New("data channel did not open")
	}
}

// Send writes one message on the open DataChannel.
func (p *Peer) Send(message []byte) error {
	p.mu.Lock()
	channel := p.channel
	p.mu.Unlock()
	if channel == nil {
		return errors.New("data channel is not available")
	}
	return channel.Send(message)
}

// Receive returns the next message or an error after timeout.
func (p *Peer) Receive(timeout time.Duration) ([]byte, error) {
	select {
	case message := <-p.messages:
		return message, nil
	case <-p.closed:
		return nil, errors.New("peer connection closed")
	case <-time.After(timeout):
		return nil, errors.New("no message received")
	}
}

// Close tears down the peer connection.
func (p *Peer) Close() {
	p.once.Do(func() {
		close(p.closed)
		p.mu.Lock()
		if p.channel != nil {
			_ = p.channel.Close()
		}
		p.mu.Unlock()
		_ = p.pc.Close()
	})
}
