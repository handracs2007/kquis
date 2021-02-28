package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/handracs2007/kquiz/telegram"
	"go.etcd.io/bbolt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func registerUser(registerer telegram.Registerer, botAPI *tgbotapi.BotAPI, chatID int64) {
	var msg tgbotapi.MessageConfig
	err := registerer.Register(chatID)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Registration failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, "Thanks for your registration.")
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to registration request. %s.\n", err)
	}
}

func unregisterUser(unregisterer telegram.Unregisterer, botAPI *tgbotapi.BotAPI, chatID int64) {
	var msg tgbotapi.MessageConfig
	err := unregisterer.Unregister(chatID)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Unregistration failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, "You have been successfully unregistered. You will not receive any future updates.")
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to unregistration request. %s.\n", err)
	}
}

func addWord(adder telegram.Adder, botAPI *tgbotapi.BotAPI, chatID int64, word string, translation string) {
	var msg tgbotapi.MessageConfig
	err := adder.Add(chatID, word, translation)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Add word failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("New word successfully added. %s -> %s.", word, translation))
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to add word request. %s.\n", err)
	}
}

func searchWord(searcher telegram.Searcher, botAPI *tgbotapi.BotAPI, chatID int64, word string) {
	var msg tgbotapi.MessageConfig
	translation, err := searcher.Search(chatID, word)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Search word failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("%s -> %s.", word, *translation))
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to search word request. %s.\n", err)
	}
}

func randomWord(searcher telegram.Searcher, botAPI *tgbotapi.BotAPI, chatID int64) []string {
	var msg tgbotapi.MessageConfig
	words, err := searcher.Random(chatID)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Get random word failed. %s.", err))
		words = nil
	} else {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("What is translation for: %s", words[0]))
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to random word request. %s.\n", err)
	}

	return words
}

func deleteWord(deleter telegram.Deleter, botAPI *tgbotapi.BotAPI, chatID int64, word string) {
	var msg tgbotapi.MessageConfig
	err := deleter.Delete(chatID, word)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Delete word failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("%s deleted.", word))
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to delete word request. %s.\n", err)
	}
}

func clearWords(deleter telegram.Deleter, botAPI *tgbotapi.BotAPI, chatID int64) {
	var msg tgbotapi.MessageConfig
	err := deleter.Clear(chatID)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Clear words failed. %s.", err))
	} else {
		msg = tgbotapi.NewMessage(chatID, "Words cleared.")
	}

	_, err = botAPI.Send(msg)
	if err != nil {
		log.Printf("Failed to respond to clear words request. %s.\n", err)
	}
}

func listWords(lister telegram.Lister, botAPI *tgbotapi.BotAPI, chatID int64) {
	var msg tgbotapi.MessageConfig
	words, err := lister.List(chatID)
	if err != nil {
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("List words failed. %s.", err))

		_, err = botAPI.Send(msg)
		if err != nil {
			log.Printf("Failed to respond to list words request. %s.\n", err)
		}
	} else {
		for _, pairs := range words {
			word := pairs[0]
			translation := pairs[1]
			msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("%s -> %s", word, translation))

			_, err = botAPI.Send(msg)
			if err != nil {
				log.Printf("Failed to respond to list words request. %s.\n", err)
			}
		}
	}
}

func main() {
	const telegramBucket = "telegram"
	const kquizBucket = "kquiz"
	const telegramToken = "1633333576:AAFQPddA8OZ6gfEVja_WHZIqJbbT9yg_I-o"

	var currRandomWord = make(map[int64]string)

	db, err := bbolt.Open("kquiz.db", 0666, nil)
	if err != nil {
		log.Fatalf("Failed to open database. %s.", err)
	}
	defer func() {
		log.Println("Closing database.")
		err = db.Close()
		if err != nil {
			log.Printf("Failed to close database. %s.", err)
		}
	}()

	// Let's create our bucket first if not exist
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(kquizBucket))
		return err
	})
	if err != nil {
		log.Printf("Failed to create bucket %s. %s.\n", kquizBucket, err)
		return
	}

	// Let's create another bucket to store our Telegram bot registrants.
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(telegramBucket))
		return err
	})
	if err != nil {
		log.Printf("Failed to create bucket %s. %s.\n", telegramBucket, err)
		return
	}

	// Let's prepare our Telegram bot
	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Printf("Failed to create telegram bot. %s.", err)
		return
	}

	botHandler := telegram.NewBotHandler(db, telegramBucket, kquizBucket)

	// Listen to Telegram updates
	go func() {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 0

		updates, err := tgBot.GetUpdatesChan(u)
		if err != nil {
			log.Printf("Failed to get updates channel. %s.", err)
			return
		}

		for update := range updates {
			if update.Message == nil {
				continue
			}

			username := update.Message.Chat.UserName
			chatID := update.Message.Chat.ID
			message := update.Message.Text
			argument := ""
			log.Printf("Received message from %s[%d]: %s\n", username, chatID, message)

			// Message can contain parameters, hence, let's get the first text before space as the message and
			// store the rest as arguments.
			if spaceIndex := strings.Index(message, " "); spaceIndex != -1 {
				argument = message[spaceIndex+1:]
				message = message[:spaceIndex]
			}

			switch message {
			case "/start", "/register":
				registerUser(botHandler, tgBot, chatID)

			case "/stop", "/unregister":
				unregisterUser(botHandler, tgBot, chatID)

			case "/add":
				if len(argument) == 0 || strings.Index(argument, " ") == -1 {
					msg := tgbotapi.NewMessage(chatID, "Please provide the Korean word and its translation.")

					_, err := tgBot.Send(msg)
					if err != nil {
						log.Printf("Failed to send response. %s.\n", err)
					}

					continue
				}

				splitted := strings.SplitN(argument, " ", 2)
				word := splitted[0]
				translation := splitted[1]

				addWord(botHandler, tgBot, chatID, word, translation)

			case "/search":
				if len(argument) == 0 {
					msg := tgbotapi.NewMessage(chatID, "Please provide the Korean word.")

					_, err := tgBot.Send(msg)
					if err != nil {
						log.Printf("Failed to send response. %s.\n", err)
					}

					continue
				}

				searchWord(botHandler, tgBot, chatID, argument)

			case "/random":
				words := randomWord(botHandler, tgBot, chatID)

				if words != nil {
					currRandomWord[chatID] = words[1]
				}

			case "/delete":
				if len(argument) == 0 {
					msg := tgbotapi.NewMessage(chatID, "Please provide the Korean word.")

					_, err := tgBot.Send(msg)
					if err != nil {
						log.Printf("Failed to send response. %s.\n", err)
					}

					continue
				}

				deleteWord(botHandler, tgBot, chatID, argument)

			case "/list":
				listWords(botHandler, tgBot, chatID)

			case "/clear":
				clearWords(botHandler, tgBot, chatID)

			default:
				// We assume this is answer from the user for the randomised word.
				answer, ok := currRandomWord[chatID]
				if !ok {
					log.Printf("Unknown command [%s].", message)
					break
				}

				var msg tgbotapi.MessageConfig
				if strings.ToLower(message) == strings.ToLower(answer) {
					msg = tgbotapi.NewMessage(chatID, "Your answer is correct")
				} else {
					msg = tgbotapi.NewMessage(chatID, fmt.Sprintf("Your answer is incorrect. Correct answer is %s.", answer))
				}

				_, err = tgBot.Send(msg)
				if err != nil {
					log.Printf("Failed to respond to answer. %s.\n", err)
				}

				delete(currRandomWord, chatID)
			}
		}
	}()

	// Make a channel that will listen to the OS signal to handle server shutdown gracefully.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	<-c // Block until signal is received from the channel.

	log.Println("Shutting down.")
}
