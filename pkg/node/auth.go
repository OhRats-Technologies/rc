package node

import (
	"strconv"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/apiclient"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

type NodeHttpAuth struct {
	DeviceID  string
	Timestamp string
	Nonce     string
	Signature string
}

func (a *NodeHttpAuth) Headers() map[string]string {
	return map[string]string{
		"x-rc-device":    a.DeviceID,
		"x-rc-timestamp": a.Timestamp,
		"x-rc-nonce":     a.Nonce,
		"x-rc-signature": a.Signature,
	}
}

func SignNodeRequest(state *NodeState, method, path string, body []byte) (*NodeHttpAuth, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := apiclient.RandomURLBytes(18)
	sig, err := crypto.SignNodeHttp(
		state.IdentitySeed,
		state.DeviceID,
		timestamp,
		nonce,
		method,
		path,
		body,
	)
	if err != nil {
		return nil, err
	}
	return &NodeHttpAuth{
		DeviceID:  state.DeviceID,
		Timestamp: timestamp,
		Nonce:     nonce,
		Signature: sig,
	}, nil
}
