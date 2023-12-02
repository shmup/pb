// permanentStore is a content-addressable storage system that uniquely identifies
// and stores content. It generates sequential alphanumeric IDs for new content,
// which are then mapped to SHA256 hashes representing the content itself.
// These hashes serve as filenames in the filesystem. When a piece of content is updated,
// its hash may change, but its ID remains consistent. If a hash collision occurs,
// indicating identical content, the existing ID is reused, avoiding duplication.
// The mapping of IDs to hashes is maintained in 'index.txt', providing persistence
// across sessions. The store supports basic CRUD operations for content snippets.
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
				// Handle the error, for example, log it, and continue with the next index
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
	ps.saveSnippet(hash, content)
	return id
}

func (ps *permanentStore) saveSnippet(hash, content string) {
	filePath := filepath.Join(baseDir, hash)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		err = os.WriteFile(filePath, []byte(content), 0644)
		if err != nil {
			panic("unable to write snippet file: " + err.Error())
		}
	}
}

func (ps *permanentStore) getSnippet(id string) (string, bool) {
	ps.RLock()
	defer ps.RUnlock()

	hash, exists := ps.index[id]
	if !exists {
		return "", false
	}

	content, err := os.ReadFile(filepath.Join(baseDir, hash))
	if err != nil {
		return "", false
	}
	return string(content), true
}

func (ps *permanentStore) updateSnippet(id, newContent string) bool {
	ps.Lock()
	defer ps.Unlock()

	hash, exists := ps.index[id]
	if !exists {
		return false
	}

	newHash := contentHash(newContent)
	if hash == newHash {
		return true // Content is identical, no need to update
	}

	// Check if another snippet is using the same hash before deleting the old file
	for _, h := range ps.index {
		if h == hash {
			os.Remove(filepath.Join(baseDir, hash)) // Only delete if no other snippet uses this hash
			break
		}
	}

	ps.index[id] = newHash
	ps.saveIndex()
	ps.saveSnippet(newHash, newContent)

	return true
}

func (ps *permanentStore) deleteSnippet(id string) bool {
	ps.Lock()
	defer ps.Unlock()

	hash, exists := ps.index[id]
	if !exists {
		return false
	}

	delete(ps.index, id)
	ps.saveIndex()
	err := os.Remove(filepath.Join(baseDir, hash))
	return err == nil
}

func contentHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil)) // Changed to hexadecimal encoding
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
