package export_ohrats_rc_transport_provider

import (
	"errors"
	"strings"

	"wit_component/ohrats_rc_transport_types"
)

const (
	maxServers   = 8
	maxURLs      = 24
	maxURLLength = 2_048
)

func planAttempts(transport string, iceServers []ohrats_rc_transport_types.IceServer) ([]ohrats_rc_transport_types.Attempt, error) {
	if transport != "webrtc" {
		return nil, errors.New("unsupported transport")
	}
	hasStun, err := hasScheme(iceServers, "stun:", "stuns:")
	if err != nil {
		return nil, err
	}
	hasTurn, err := hasScheme(iceServers, "turn:", "turns:")
	if err != nil {
		return nil, err
	}
	attempts := []ohrats_rc_transport_types.Attempt{
		attempt(
			ohrats_rc_transport_types.IceMode_Host,
			ohrats_rc_transport_types.RouteClass_DirectHost,
			2_000,
			6_000,
			0,
		),
	}
	if hasStun {
		attempts = append(attempts, attempt(
			ohrats_rc_transport_types.IceMode_Stun,
			ohrats_rc_transport_types.RouteClass_DirectStun,
			8_000,
			12_000,
			1_200,
		))
	}
	if hasTurn {
		attempts = append(attempts, attempt(
			ohrats_rc_transport_types.IceMode_Relay,
			ohrats_rc_transport_types.RouteClass_TurnRelay,
			15_000,
			18_000,
			1_200,
		))
	}
	return attempts, nil
}

func planAnswer(transport string, request ohrats_rc_transport_types.AnswerRequest) (ohrats_rc_transport_types.AnswerPlan, error) {
	if transport != "webrtc" {
		return ohrats_rc_transport_types.AnswerPlan{}, errors.New("unsupported transport")
	}
	if len(request.IceServers) > maxServers {
		return ohrats_rc_transport_types.AnswerPlan{}, errors.New("too many ICE servers")
	}
	var totalURLs int
	servers := make([]ohrats_rc_transport_types.IceServer, 0, len(request.IceServers))
	for _, server := range request.IceServers {
		filtered, err := filterURLs(request.Mode, server.Urls)
		if err != nil {
			return ohrats_rc_transport_types.AnswerPlan{}, err
		}
		totalURLs += len(filtered)
		if totalURLs > maxURLs {
			return ohrats_rc_transport_types.AnswerPlan{}, errors.New("too many ICE URLs")
		}
		if len(filtered) != 0 {
			servers = append(servers, ohrats_rc_transport_types.IceServer{
				Urls:       filtered,
				Username:   server.Username,
				Credential: server.Credential,
			})
		}
	}
	gatherTimeoutMs, connectTimeoutMs := timeouts(request.Mode)
	return ohrats_rc_transport_types.AnswerPlan{
		IceServers:       servers,
		GatherTimeoutMs:  gatherTimeoutMs,
		ConnectTimeoutMs: connectTimeoutMs,
	}, nil
}

func attempt(mode uint8, route uint8, gatherTimeoutMs uint32, connectTimeoutMs uint32, retryDelayMs uint32) ohrats_rc_transport_types.Attempt {
	return ohrats_rc_transport_types.Attempt{
		Mode:             mode,
		Route:            route,
		GatherTimeoutMs:  gatherTimeoutMs,
		ConnectTimeoutMs: connectTimeoutMs,
		RetryDelayMs:     retryDelayMs,
	}
}

func hasScheme(servers []ohrats_rc_transport_types.IceServer, schemes ...string) (bool, error) {
	count := 0
	for _, server := range servers {
		for _, url := range server.Urls {
			count++
			if count > maxURLs || len(url) > maxURLLength || strings.ContainsAny(url, "\x00\r\n") {
				return false, errors.New("invalid ICE server list")
			}
			lower := asciiLower(strings.TrimSpace(url))
			for _, scheme := range schemes {
				if strings.HasPrefix(lower, scheme) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func filterURLs(mode uint8, urls []string) ([]string, error) {
	filtered := make([]string, 0, len(urls))
	for _, url := range urls {
		value := strings.TrimSpace(url)
		if len(value) > maxURLLength || strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("invalid ICE URL")
		}
		lower := asciiLower(value)
		include := false
		switch mode {
		case ohrats_rc_transport_types.IceMode_Stun:
			include = strings.HasPrefix(lower, "stun:") || strings.HasPrefix(lower, "stuns:")
		case ohrats_rc_transport_types.IceMode_Relay:
			include = strings.HasPrefix(lower, "turn:") || strings.HasPrefix(lower, "turns:")
		}
		if include {
			filtered = append(filtered, value)
		}
	}
	return filtered, nil
}

func timeouts(mode uint8) (uint32, uint32) {
	switch mode {
	case ohrats_rc_transport_types.IceMode_Stun:
		return 8_000, 12_000
	case ohrats_rc_transport_types.IceMode_Relay:
		return 15_000, 18_000
	default:
		return 2_000, 6_000
	}
}

func asciiLower(value string) string {
	lower := make([]byte, len(value))
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		lower[index] = character
	}
	return string(lower)
}
