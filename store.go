// Package main implements a thread-safe permanent storage system for managing
// text snippets. It features an index to track stored snippets by unique IDs,
// file-based persistence, and content deduplication using SHA-256 hashing.
// Supports create, read, update, and delete (CRUD) operations.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	baseDir           = "data"
	idChars           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	indexFileName     = "index.txt"
	ownersFileName    = "owners.txt"
	passwordsFileName = "passwords.txt"
)

type Store struct {
	sync.RWMutex
	index     map[string]string
	owners    map[string]string
	passwords map[string]string
}

func newStore() *Store {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		panic("unable to create base directory: " + err.Error())
	}

	return &Store{
		index:     loadMapFromFile(indexFileName),
		owners:    loadMapFromFile(ownersFileName),
		passwords: loadMapFromFile(passwordsFileName),
	}
}

func loadMapFromFile(filename string) map[string]string {
	content, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		panic("unable to read file " + filename + ": " + err.Error())
	}

	result := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[0] != "" {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func (ps *Store) saveToFile(data map[string]string, filename string) {
	var sb strings.Builder
	for id, value := range data {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(value)
		sb.WriteString("\n")
	}

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		panic("unable to write file " + filename + ": " + err.Error())
	}
}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

func (ps *Store) generateID() string {
	ps.Lock()
	defer ps.Unlock()

	length := 1
	var indices []int

	for {
		possibleIDs := intPow(len(idChars), length)
		if len(indices) == 0 {
			indices = rand.Perm(possibleIDs)
		}

		for _, idx := range indices {
			id, err := baseN(idx, idChars, length)
			if err != nil {
				log.Println(err)
				continue
			}

			if _, exists := ps.index[id]; !exists {
				return id
			}
		}

		length++
		indices = nil
	}
}

func (ps *Store) createSnippet(content string, owner string, password string) string {
	hash := contentHash(content)

	ps.RLock()
	for id, existingHash := range ps.index {
		if existingHash == hash {
			if owner != "" && (ps.owners[id] == "" || (ps.owners[id] == owner && ps.passwords[id] == password)) {
				ps.RUnlock()
				ps.Lock()
				ps.owners[id] = owner
				ps.passwords[id] = password
				ps.Unlock()
				ps.saveToFile(ps.owners, ownersFileName)
				ps.saveToFile(ps.passwords, passwordsFileName)
				return id
			}
			ps.RUnlock()
			return id
		}
	}
	ps.RUnlock()

	id := ps.generateID()
	ps.Lock()
	ps.index[id] = hash
	if owner != "" {
		ps.owners[id] = owner
		ps.passwords[id] = password
	}
	ps.Unlock()

	ps.saveToFile(ps.index, indexFileName)
	ps.saveToFile(ps.owners, ownersFileName)
	ps.saveToFile(ps.passwords, passwordsFileName)

	filePath := filepath.Join(baseDir, id)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		panic("unable to write snippet file: " + err.Error())
	}

	return id
}

func (ps *Store) getSnippet(id string) (string, bool) {
	ps.RLock()
	_, exists := ps.index[id]
	ps.RUnlock()

	if !exists {
		return "", false
	}

	content, err := os.ReadFile(filepath.Join(baseDir, id))
	if err != nil {
		return "", false
	}
	return string(content), true
}

func (ps *Store) deleteSnippet(id string, username string, password string) bool {
	ps.Lock()
	defer ps.Unlock()

	_, exists := ps.index[id]
	if !exists {
		return false
	}

	owner, hasOwner := ps.owners[id]
	storedPassword, hasPassword := ps.passwords[id]

	if hasOwner {
		if username == "" {
			return false
		}

		if owner != username || (hasPassword && storedPassword != password) {
			return false
		}
	}

	delete(ps.index, id)
	delete(ps.owners, id)
	delete(ps.passwords, id)

	ps.saveToFile(ps.index, indexFileName)
	ps.saveToFile(ps.owners, ownersFileName)
	ps.saveToFile(ps.passwords, passwordsFileName)

	go func() {
		if err := os.Remove(filepath.Join(baseDir, id)); err != nil {
			log.Printf("Failed to remove file: %v", err)
		}
	}()

	return true
}

func (ps *Store) updateSnippet(id, newContent string, username string, password string) bool {
	ps.Lock()
	defer ps.Unlock()

	_, exists := ps.index[id]
	if !exists {
		return false
	}

	if username != "" {
		owner, hasOwner := ps.owners[id]
		storedPassword, hasPassword := ps.passwords[id]

		if hasOwner && (owner != username || (hasPassword && storedPassword != password)) {
			return false
		}
	}

	newHash := contentHash(newContent)
	oldHash := ps.index[id]
	if oldHash == newHash {
		return true
	}

	ps.index[id] = newHash
	if username != "" {
		ps.owners[id] = username
		ps.passwords[id] = password
	}

	ps.saveToFile(ps.index, indexFileName)
	ps.saveToFile(ps.owners, ownersFileName)
	ps.saveToFile(ps.passwords, passwordsFileName)

	if err := os.WriteFile(filepath.Join(baseDir, id), []byte(newContent), 0644); err != nil {
		log.Printf("Failed to write updated file: %v", err)
		return false
	}

	return true
}

func contentHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

func intPow(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func baseN(num int, chars string, length int) (string, error) {
	base := len(chars)
	maxNum := intPow(base, length) - 1
	if num > maxNum {
		return "", fmt.Errorf("number %d too large to encode with length %d in base %d", num, length, base)
	}

	res := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		res[i] = chars[num%base]
		num /= base
	}
	return string(res), nil
}
