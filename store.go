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
	passwordsFileName = "passwords.txt" // New file to track passwords
)

func loadPasswords() map[string]string {
	content, err := os.ReadFile(passwordsFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		panic("unable to read passwords file: " + err.Error())
	}

	lines := strings.Split(string(content), "\n")
	passwords := make(map[string]string)
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[0] != "" {
			passwords[parts[0]] = parts[1]
		}
	}
	return passwords
}

func (ps *Store) savePasswords() {
	var sb strings.Builder
	for id, password := range ps.passwords {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(password)
		sb.WriteString("\n")
	}

	err := os.WriteFile(passwordsFileName, []byte(sb.String()), 0644)
	if err != nil {
		panic("unable to write passwords file: " + err.Error())
	}
}

type Store struct {
	sync.RWMutex
	index     map[string]string
	owners    map[string]string
	passwords map[string]string // Add passwords map to track passwords per snippet
}

func newStore() *Store {
	ps := &Store{
		index:     loadIndex(),
		owners:    loadOwners(),
		passwords: loadPasswords(),
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		panic("unable to create base directory for storage: " + err.Error())
	}
	return ps
}

func loadIndex() map[string]string {
	content, err := os.ReadFile(indexFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		panic("unable to read index file: " + err.Error())
	}

	lines := strings.Split(string(content), "\n")
	index := make(map[string]string)
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[0] != "" {
			index[parts[0]] = parts[1]
		}
	}
	return index
}

func loadOwners() map[string]string {
	content, err := os.ReadFile(ownersFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		panic("unable to read owners file: " + err.Error())
	}

	lines := strings.Split(string(content), "\n")
	owners := make(map[string]string)
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[0] != "" {
			owners[parts[0]] = parts[1]
		}
	}
	return owners
}

func (ps *Store) saveIndex() {
	var sb strings.Builder
	for id, hash := range ps.index {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(hash)
		sb.WriteString("\n")
	}

	err := os.WriteFile(indexFileName, []byte(sb.String()), 0644)
	if err != nil {
		panic("unable to write index file: " + err.Error())
	}
}

func (ps *Store) saveOwners() {
	var sb strings.Builder
	for id, owner := range ps.owners {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(owner)
		sb.WriteString("\n")
	}

	err := os.WriteFile(ownersFileName, []byte(sb.String()), 0644)
	if err != nil {
		panic("unable to write owners file: " + err.Error())
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
				indices = indices[1:]
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
			// if finding existing content, check if we should update ownership
			if owner != "" && (ps.owners[id] == "" || (ps.owners[id] == owner && ps.passwords[id] == password)) {
				ps.RUnlock()
				ps.Lock()
				ps.owners[id] = owner
				ps.passwords[id] = password
				ps.Unlock()
				ps.saveOwners()
				ps.savePasswords()
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
	ps.saveIndex()
	ps.saveOwners()
	ps.savePasswords()
	ps.saveSnippet(id, content)
	return id
}

func (ps *Store) saveSnippet(id, content string) {
	filePath := filepath.Join(baseDir, id)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		panic("unable to write snippet file: " + err.Error())
	}
}

func (ps *Store) getSnippet(id string) (string, bool) {
	ps.RLock()
	defer ps.RUnlock()

	_, exists := ps.index[id]
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
	_, exists := ps.index[id]
	if !exists {
		ps.Unlock()
		return false
	}

	// Check if the snippet has an owner
	owner, hasOwner := ps.owners[id]
	storedPassword, hasPassword := ps.passwords[id]

	// If it has an owner, authentication is required
	if hasOwner {
		if username == "" {
			// No authentication provided
			ps.Unlock()
			return false
		}

		// Strict check: require username match AND the ORIGINAL password match
		if owner != username || (hasPassword && storedPassword != password) {
			// Either wrong username or the password doesn't match what was stored originally
			ps.Unlock()
			return false
		}
	}

	delete(ps.index, id)
	delete(ps.owners, id)
	delete(ps.passwords, id)

	// Save before unlocking to prevent race conditions
	var sb strings.Builder

	// Save index
	for id, hash := range ps.index {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(hash)
		sb.WriteString("\n")
	}
	os.WriteFile(indexFileName, []byte(sb.String()), 0644)

	// Save owners
	sb.Reset()
	for id, owner := range ps.owners {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(owner)
		sb.WriteString("\n")
	}
	os.WriteFile(ownersFileName, []byte(sb.String()), 0644)

	// Save passwords
	sb.Reset()
	for id, password := range ps.passwords {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(password)
		sb.WriteString("\n")
	}
	os.WriteFile(passwordsFileName, []byte(sb.String()), 0644)

	ps.Unlock()

	go func() {
		if err := os.Remove(filepath.Join(baseDir, id)); err != nil {
			log.Printf("Failed to remove file: %v", err)
		}
	}()

	return true
}

func (ps *Store) updateSnippet(id, newContent string, username string, password string) bool {
	ps.Lock()
	_, exists := ps.index[id]
	if !exists {
		ps.Unlock()
		return false
	}

	// Check ownership and password if username is provided
	if username != "" {
		owner, hasOwner := ps.owners[id]
		storedPassword, hasPassword := ps.passwords[id]

		// Strict check: require exact match with the ORIGINAL stored password
		if hasOwner && (owner != username || (hasPassword && storedPassword != password)) {
			ps.Unlock()
			return false // not the owner or doesn't match original password
		}
	}

	newHash := contentHash(newContent)
	oldHash := ps.index[id]
	if oldHash == newHash {
		ps.Unlock()
		return true
	}

	ps.index[id] = newHash
	// update owner if authenticated
	if username != "" {
		ps.owners[id] = username
		ps.passwords[id] = password // Update to the new password
	}
	ps.Unlock()

	ps.saveIndex()
	ps.saveOwners()
	ps.savePasswords()
	ps.saveSnippet(id, newContent)

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
