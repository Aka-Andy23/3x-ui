package clientconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	MaxDocumentBytes = 16 << 20
	MaxDepth         = 64
	MaxNodes         = 200000
	MaxConfigs       = 128
	MaxOutbounds     = 512
)

type Config struct {
	Remark    string
	Outbounds []map[string]any
	Warnings  []string
}

type ParseError struct {
	Message string
	Line    int
	Column  int
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s at line %d, column %d", e.Message, e.Line, e.Column)
	}
	return e.Message
}

var protocols = map[string]struct{}{
	"vmess":       {},
	"vless":       {},
	"trojan":      {},
	"shadowsocks": {},
	"socks":       {},
	"http":        {},
	"wireguard":   {},
	"hysteria":    {},
}

var outboundFields = map[string]struct{}{
	"tag":            {},
	"protocol":       {},
	"settings":       {},
	"streamSettings": {},
	"mux":            {},
}

var knownTopLevel = map[string]struct{}{
	"outbounds":        {},
	"remarks":          {},
	"meta":             {},
	"dns":              {},
	"routing":          {},
	"inbounds":         {},
	"log":              {},
	"api":              {},
	"policy":           {},
	"stats":            {},
	"reverse":          {},
	"transport":        {},
	"observatory":      {},
	"burstObservatory": {},
	"metrics":          {},
	"fakedns":          {},
}

func Parse(data []byte) ([]Config, error) {
	if len(data) == 0 {
		return nil, errors.New("JSON document is empty")
	}
	if len(data) > MaxDocumentBytes {
		return nil, errors.New("JSON document exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, syntaxError(data, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON document contains multiple root values")
	}
	if err := validateShape(root, 0, new(int)); err != nil {
		return nil, err
	}
	var objects []map[string]any
	switch value := root.(type) {
	case map[string]any:
		objects = []map[string]any{value}
	case []any:
		if len(value) > MaxConfigs {
			return nil, errors.New("JSON document contains too many configurations")
		}
		for _, item := range value {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("configuration array may contain only objects")
			}
			objects = append(objects, obj)
		}
	default:
		return nil, errors.New("JSON root must be an object or array of objects")
	}
	out := make([]Config, 0, len(objects))
	for _, obj := range objects {
		cfg, err := sanitizeConfig(obj)
		if err != nil {
			return nil, err
		}
		if len(cfg.Outbounds) > 0 {
			out = append(out, cfg)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("JSON document contains no supported proxy outbounds")
	}
	return out, nil
}

func sanitizeConfig(obj map[string]any) (Config, error) {
	cfg := Config{}
	if remark, ok := obj["remarks"].(string); ok {
		cfg.Remark = strings.TrimSpace(remark)
	}
	for key := range obj {
		if _, ok := knownTopLevel[key]; !ok {
			return Config{}, fmt.Errorf("unsupported top-level section %q", key)
		}
		switch key {
		case "api", "inbounds", "log", "policy", "stats", "reverse", "transport", "metrics":
			cfg.Warnings = append(cfg.Warnings, "stripped "+key)
		}
	}
	rawOutbounds, ok := obj["outbounds"].([]any)
	if !ok {
		return cfg, nil
	}
	if len(rawOutbounds) > MaxOutbounds {
		return Config{}, errors.New("configuration contains too many outbounds")
	}
	for _, raw := range rawOutbounds {
		outbound, ok := raw.(map[string]any)
		if !ok {
			return Config{}, errors.New("outbound must be an object")
		}
		protocol, _ := outbound["protocol"].(string)
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		if _, ok := protocols[protocol]; !ok {
			continue
		}
		clean := make(map[string]any)
		for key, value := range outbound {
			if _, ok := outboundFields[key]; ok {
				clean[key] = value
			}
		}
		clean["protocol"] = protocol
		if _, ok := clean["settings"].(map[string]any); !ok {
			return Config{}, fmt.Errorf("outbound protocol %q requires object settings", protocol)
		}
		if tag, ok := clean["tag"].(string); ok {
			clean["tag"] = strings.TrimSpace(tag)
		}
		cfg.Outbounds = append(cfg.Outbounds, clean)
	}
	return cfg, nil
}

func validateShape(value any, depth int, nodes *int) error {
	if depth > MaxDepth {
		return errors.New("JSON document exceeds nesting limit")
	}
	(*nodes)++
	if *nodes > MaxNodes {
		return errors.New("JSON document exceeds complexity limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if len(key) > 4096 {
				return errors.New("JSON object key exceeds length limit")
			}
			if err := validateShape(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateShape(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > 1<<20 {
			return errors.New("JSON string exceeds length limit")
		}
	}
	return nil
}

func syntaxError(data []byte, err error) error {
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		return &ParseError{Message: "invalid JSON"}
	}
	offset := int(syntax.Offset)
	if offset < 1 {
		offset = 1
	}
	if offset > len(data) {
		offset = len(data)
	}
	before := data[:offset]
	line := bytes.Count(before, []byte{'\n'}) + 1
	lastNewline := bytes.LastIndexByte(before, '\n')
	column := offset
	if lastNewline >= 0 {
		column = offset - lastNewline - 1
	}
	if column < 1 {
		column = 1
	}
	return &ParseError{Message: "invalid JSON", Line: line, Column: column}
}
