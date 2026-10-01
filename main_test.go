package main

import (
	"errors"
	"testing"
)

func TestTunnelModelRecordsConnection(t *testing.T) {
	model := newTunnelModel("https://karo.salkaro.com", "http://localhost:3000", nil, nil)
	model.recordLog("INF Registered tunnel connection connIndex=0")

	if model.status != "CONNECTED" {
		t.Fatalf("status = %q, want CONNECTED", model.status)
	}
	if model.lastActivity != "Cloudflare edge connection registered." {
		t.Fatalf("lastActivity = %q", model.lastActivity)
	}
}

func TestTunnelModelKeepsRecentLogs(t *testing.T) {
	model := newTunnelModel("https://karo.salkaro.com", "http://localhost:3000", nil, nil)
	for i := 0; i < maxVisibleLogs+2; i++ {
		model.recordLog("log line " + string(rune('a'+i)))
	}

	if len(model.logs) != maxVisibleLogs {
		t.Fatalf("log count = %d, want %d", len(model.logs), maxVisibleLogs)
	}
	if model.logs[0] != "log line c" {
		t.Fatalf("first log = %q, want %q", model.logs[0], "log line c")
	}
}

func TestTunnelModelRetainsFailureDetails(t *testing.T) {
	model := newTunnelModel("https://karo.salkaro.com", "http://localhost:3000", nil, nil)
	updated, _ := model.Update(tunnelDoneMsg{err: errors.New("exit status 1")})
	result := updated.(tunnelModel)

	if !result.completed {
		t.Fatal("completed = false, want true")
	}
	if result.status != "FAILED" {
		t.Fatalf("status = %q, want FAILED", result.status)
	}
	if result.processErr == nil {
		t.Fatal("processErr = nil, want an error")
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{name: "http", args: []string{"http", "3000"}, want: options{Protocol: "http", Port: 3000}},
		{name: "https", args: []string{"https", "8443"}, want: options{Protocol: "https", Port: 8443}},
		{name: "missing port", args: []string{"http"}, wantErr: true},
		{name: "extra argument", args: []string{"http", "3000", "extra"}, wantErr: true},
		{name: "unsupported protocol", args: []string{"tcp", "3000"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseArgs(test.args)
			if test.wantErr {
				if err == nil {
					t.Fatal("parseArgs() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseArgs() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		value   string
		want    int
		wantErr bool
	}{
		{value: "1", want: 1},
		{value: "65535", want: 65535},
		{value: "0", wantErr: true},
		{value: "65536", wantErr: true},
		{value: "-1", wantErr: true},
		{value: "three-thousand", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := validatePort(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatal("validatePort() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("validatePort() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("validatePort() = %d, want %d", got, test.want)
			}
		})
	}
}
