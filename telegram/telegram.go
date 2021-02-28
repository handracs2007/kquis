package telegram

import (
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"log"
	"math/rand"
	"strings"
	"time"
)

// ErrAlreadyRegistered indicates that the user has been registered.
var ErrAlreadyRegistered = errors.New("already registered")

// ErrNotRegistered indicates that the user has not yet registered.
var ErrNotRegistered = errors.New("not yet registered")

// ErrDatabaseError indicates that there is a database error occurred.
var ErrDatabaseError = errors.New("database error")

// ErrDuplicateWord indicates that the word has been added.
var ErrDuplicateWord = errors.New("duplicate word")

// ErrWordNotFound indicates that the word not found.
var ErrWordNotFound = errors.New("word not found")

// Registerer defines operations to be fulfilled by the implementation that has capability to register user.
type Registerer interface {
	Register(chatID int64) error
}

// Checker defines operations to be fulfilled by the implementation that has capability to check if an ID is registered.
type Checker interface {
	IsRegistered(chatID int64) bool
	IsAdded(chatID int64, word string) bool
}

// Unregisterer defines operations to be fulfilled by the implementation that has capability to unregister user.
type Unregisterer interface {
	Unregister(chatID int64) error
}

// Adder defines operations to be fulfilled by the implementation that has capability to add word.
type Adder interface {
	Add(chatID int64, word string, translation string) error
}

// Deleter defines operations to be fulfilled by the implementation that has capability to delete word.
type Deleter interface {
	Delete(chatID int64, word string) error
	Clear(chatID int64) error
}

// Searcher defines operations to be fulfilled by the implementation that has capability to search a word.
type Searcher interface {
	Search(chatID int64, word string) (*string, error)
	Random(chatID int64) ([]string, error)
}

// Lister defines operations to be fulfilled by the implementation that has capability to list words.
type Lister interface {
	List(chatID int64) ([][]string, error)
}

// BotHandler handles Telegram bot operations.
type BotHandler struct {
	telegramBucket []byte
	kquizBucket    []byte
	db             *bbolt.DB
}

// NewBotHandler creates a new instance of BotHandler
func NewBotHandler(db *bbolt.DB, telegramBucket string, kquizBucket string) BotHandler {
	return BotHandler{db: db, telegramBucket: []byte(telegramBucket), kquizBucket: []byte(kquizBucket)}
}

func (bot BotHandler) IsRegistered(chatID int64) bool {
	exists := false

	err := bot.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bot.telegramBucket)
		data := bucket.Get([]byte(fmt.Sprintf("%d", chatID)))
		exists = data != nil

		return nil
	})
	if err != nil {
		log.Printf("Failed to read data from telegram bucket. %s.\n", err)
	}

	return exists
}

func (bot BotHandler) IsAdded(chatID int64, word string) bool {
	exists := false

	err := bot.db.View(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d%s", chatID, word))
		bucket := tx.Bucket(bot.kquizBucket)
		data := bucket.Get(key)
		exists = data != nil

		return nil
	})
	if err != nil {
		log.Printf("Failed to read data from telegram bucket. %s.\n", err)
	}

	return exists
}

// Register registers a new user. This function can return the following errors:
//  - ErrAlreadyRegistered
//  - ErrDatabaseError
func (bot BotHandler) Register(chatID int64) error {
	if bot.IsRegistered(chatID) {
		return ErrAlreadyRegistered
	}

	err := bot.db.Update(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d", chatID))
		value := key

		bucket := tx.Bucket(bot.telegramBucket)
		return bucket.Put(key, value)
	})
	if err != nil {
		log.Printf("Failed to update registration data. %s.\n", err)
		return ErrDatabaseError
	}

	return nil
}

// Unregister unregisters an existing user. This function can return the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
func (bot BotHandler) Unregister(chatID int64) error {
	if !bot.IsRegistered(chatID) {
		return ErrNotRegistered
	}

	err := bot.db.Update(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d", chatID))

		bucket := tx.Bucket(bot.telegramBucket)
		return bucket.Delete(key)
	})
	if err != nil {
		log.Printf("Failed to update registration data. %s.\n", err)
		return ErrDatabaseError
	}

	return nil
}

// Add adds a word and its translation to the database. This data is unique for each user identified by the chat ID.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
//  - ErrDuplicateWord
func (bot BotHandler) Add(chatID int64, word string, translation string) error {
	if !bot.IsRegistered(chatID) {
		return ErrNotRegistered
	}

	if bot.IsAdded(chatID, word) {
		return ErrDuplicateWord
	}

	err := bot.db.Update(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d%s", chatID, word))
		bucket := tx.Bucket(bot.kquizBucket)
		return bucket.Put(key, []byte(translation))
	})
	if err != nil {
		log.Printf("Failed to add word. %s.", err)
		return ErrDatabaseError
	}

	return nil
}

// Search searches a word from the database.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
//  - ErrWordNotFound
func (bot BotHandler) Search(chatID int64, word string) (*string, error) {
	if !bot.IsRegistered(chatID) {
		return nil, ErrNotRegistered
	}

	var translation []byte

	err := bot.db.View(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d%s", chatID, word))
		bucket := tx.Bucket(bot.kquizBucket)
		translation = bucket.Get(key)

		if translation == nil {
			return ErrWordNotFound
		}

		return nil
	})
	if err != nil {
		log.Printf("Failed to get word. %s.", err)

		if err != ErrWordNotFound {
			return nil, ErrDatabaseError
		} else {
			return nil, ErrWordNotFound
		}
	}

	translationStr := string(translation)
	return &translationStr, nil
}

// Random gets random item from the database. When successful, this returned slice will contain
// 2 elements; first element is the Korean word and the second element is the translation.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
//  - ErrWordNotFound
func (bot BotHandler) Random(chatID int64) ([]string, error) {
	if !bot.IsRegistered(chatID) {
		return nil, ErrNotRegistered
	}

	items, err := bot.List(chatID)
	if err != nil {
		log.Printf("Failed to get random word. %s.", err)
		return nil, err
	}

	rand.Seed(time.Now().UnixNano())
	idx := rand.Intn(len(items))

	return items[idx], nil
}

// Delete deletes a word from the database.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
//  - ErrWordNotFound
func (bot BotHandler) Delete(chatID int64, word string) error {
	if !bot.IsRegistered(chatID) {
		return ErrNotRegistered
	}

	if !bot.IsAdded(chatID, word) {
		return ErrWordNotFound
	}

	err := bot.db.Update(func(tx *bbolt.Tx) error {
		key := []byte(fmt.Sprintf("%d%s", chatID, word))
		bucket := tx.Bucket(bot.kquizBucket)
		return bucket.Delete(key)
	})
	if err != nil {
		log.Printf("Failed to get word. %s.", err)
		return ErrDatabaseError
	}

	return nil
}

// Clear clears all words from the database owned by the user identified with chat ID.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
func (bot BotHandler) Clear(chatID int64) error {
	if !bot.IsRegistered(chatID) {
		return ErrNotRegistered
	}

	err := bot.db.Update(func(tx *bbolt.Tx) error {
		chatIDStr := fmt.Sprintf("%d", chatID)
		bucket := tx.Bucket(bot.kquizBucket)
		cursor := bucket.Cursor()

		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			keyStr := string(key)
			if !strings.HasPrefix(keyStr, chatIDStr) {
				// This word is not owned by the user. Skip.
				continue
			}

			err := cursor.Delete()
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		log.Printf("Failed to clear words. %s.", err)
		return ErrDatabaseError
	}

	return nil
}

// List lists words from the database owned by the user as identified by the chat ID.
// This function returns the following errors:
//  - ErrNotRegistered
//  - ErrDatabaseError
//  - ErrWordNotFound
func (bot BotHandler) List(chatID int64) ([][]string, error) {
	wordMap := make([][]string, 0)

	if !bot.IsRegistered(chatID) {
		return nil, ErrNotRegistered
	}

	err := bot.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bot.kquizBucket)
		cursor := bucket.Cursor()

		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if !strings.HasPrefix(string(key), fmt.Sprintf("%d", chatID)) {
				continue
			}

			// Remove the chatID from the koreanWord
			koreanWord := strings.ReplaceAll(string(key), fmt.Sprintf("%d", chatID), "")
			translation := string(value)
			wordMap = append(wordMap, []string{koreanWord, translation})
		}

		return nil
	})
	if err != nil {
		log.Printf("Failed to list words. %s.", err)
		return nil, ErrDatabaseError
	}

	if len(wordMap) == 0 {
		return nil, ErrWordNotFound
	}

	return wordMap, nil
}
