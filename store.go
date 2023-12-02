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
	indexFileName = "index.txt"
	baseDir       = "data"
	idChars       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

type permanentStore struct {
	sync.RWMutex
	index map[string]string
}

func newPermanentStore() *permanentStore {
	ps := &permanentStore{
		index: loadIndex(),
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
		if len(parts) == 2 {
			index[parts[0]] = parts[1]
		}
	}
	return index
}

func (ps *permanentStore) saveIndex() {
	ps.Lock()
	defer ps.Unlock()

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

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (ps *permanentStore) generateID() string {
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

func (ps *permanentStore) createSnippet(content string) string {
	hash := contentHash(content)

	ps.RLock()
	for id, existingHash := range ps.index {
		if existingHash == hash {
			ps.RUnlock()
			return id
		}
	}
	ps.RUnlock()

	id := ps.generateID()
	ps.Lock()
	ps.index[id] = hash
	ps.Unlock()
	ps.saveIndex()
	ps.saveSnippet(id, content)
	return id
}

func (ps *permanentStore) saveSnippet(id, content string) {
	filePath := filepath.Join(baseDir, id)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		panic("unable to write snippet file: " + err.Error())
	}
}

func (ps *permanentStore) getSnippet(id string) (string, bool) {
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

func (ps *permanentStore) updateSnippet(id, newContent string) bool {
	ps.Lock()
	_, exists := ps.index[id]
	if !exists {
		ps.Unlock()
		return false
	}
	newHash := contentHash(newContent)
	oldHash := ps.index[id]
	if oldHash == newHash {
		ps.Unlock()
		return true
	}

	ps.index[id] = newHash
	ps.Unlock()

	ps.saveIndex()
	ps.saveSnippet(id, newContent)

	return true
}

func (ps *permanentStore) deleteSnippet(id string) bool {
	ps.Lock()
	defer ps.Unlock()

	_, exists := ps.index[id]
	if !exists {
		return false
	}

	delete(ps.index, id)
	ps.saveIndex()
	err := os.Remove(filepath.Join(baseDir, id))
	return err == nil
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
