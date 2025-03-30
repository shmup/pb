package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func authenticateUser(r *http.Request) (string, string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", "", false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}

	netrcData, err := os.ReadFile(filepath.Join(homeDir, ".netrc"))
	if err != nil {
		return "", "", false
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
				if currentMachine == r.Host {
					netrcUser = fields[i+1]
				}
			case "password":
				if currentMachine == r.Host {
					netrcPass = fields[i+1]
				}
			}
		}
	}

	return username, password, username == netrcUser && password == netrcPass
}
