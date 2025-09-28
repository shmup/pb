package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func authenticateUser(r *http.Request) (string, string, bool) {
	username, password, ok := r.BasicAuth()
	if ok {
		if validateCredentialsNetrc(r.Host, username, password) {
			return username, password, true
		}
		return username, password, false
	}

	if userInfo := r.URL.User; userInfo != nil {
		username = userInfo.Username()
		password, _ = userInfo.Password()
		if username != "" {
			if validateCredentialsNetrc(r.Host, username, password) {
				return username, password, true
			}
			return username, password, false
		}
	}

	return "", "", false
}

func validateCredentialsNetrc(host, username, password string) bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	netrcPath := filepath.Join(homeDir, ".netrc")
	netrcData, err := os.ReadFile(netrcPath)
	if err != nil {
		return false
	}

	var currentMachine, netrcUser, netrcPass string
	for _, line := range strings.Split(string(netrcData), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}

		for i := 0; i < len(fields)-1; i++ {
			switch fields[i] {
			case "machine":
				currentMachine = fields[i+1]
			case "login":
				if currentMachine == host {
					netrcUser = fields[i+1]
				}
			case "password":
				if currentMachine == host {
					netrcPass = fields[i+1]
				}
			}
		}
	}

	return username == netrcUser && password == netrcPass
}
