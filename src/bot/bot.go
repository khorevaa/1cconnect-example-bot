package bot

import (
	"encoding/json"
	"errors"
	"github.com/go-redis/redis/v7"
	"net/http"
	"path/filepath"
	"strings"
	"сonnect-companion/database"

	"сonnect-companion/bot/messages"
	"сonnect-companion/bot/requests"
	"сonnect-companion/config"
	"сonnect-companion/logger"

	"github.com/gin-gonic/gin"
)

const (
	BOT_PHRASE_GREETING = "Выберите, какая информация вас интересует:"
	BOT_PHRASE_SORRY    = "Извините, но я вас не понимаю. Выберите, пожалуйста, один из вариантов:"
	BOT_PHRASE_OK       = "Вот. пожалуйста"
)

var (
	cnf = &config.Conf{}
)

func Configure(c *config.Conf) {
	cnf = c
}

func Receive(c *gin.Context) {
	var msg messages.Message
	if err := c.BindJSON(&msg); err != nil {
		logger.Warning("Error while receive message", err)

		c.Status(http.StatusBadRequest)
		return
	}

	logger.Debug("Receive message:", msg)

	go func(msg messages.Message) {
		chatState := getState(c, &msg)

		newState, err := processMessage(&msg, &chatState)

		err = changeState(c, &msg, &chatState, newState)

		if err != nil {

		}
	}(msg)

	c.Status(http.StatusOK)
}

func getState(c *gin.Context, msg *messages.Message) database.Chat {
	db := c.MustGet("db").(*redis.Client)

	var chatState database.Chat

	dbStateKey := database.PREFIX_STATE + msg.UserId.String() + ":" + msg.LineId.String()

	dbStateRaw, err := db.Get(dbStateKey).Bytes()
	if err == redis.Nil {
		logger.Info("No state in db for " + msg.UserId.String() + ":" + msg.LineId.String())

		chatState = database.Chat{
			PreviousState: database.STATE_GREETINGS,
			CurrentState:  database.STATE_GREETINGS,
		}
	} else if err != nil {
		logger.Warning("Error while reading state from redis", err)
	} else {
		err = json.Unmarshal(dbStateRaw, &chatState)
		if err != nil {
			logger.Warning("Error while decoding state", err)
		}
	}

	return chatState
}

func changeState(c *gin.Context, msg *messages.Message, chatState *database.Chat, toState database.ChatState) error {
	db := c.MustGet("db").(*redis.Client)

	chatState.PreviousState = chatState.CurrentState
	chatState.CurrentState = toState

	data, err := json.Marshal(chatState)
	if err != nil {
		logger.Warning("Error while change state to db", err)

		return err
	}

	dbStateKey := database.PREFIX_STATE + msg.UserId.String() + ":" + msg.LineId.String()

	result, err := db.Set(dbStateKey, data, database.EXPIRE).Result()
	logger.Debug("Write state to db result", result)
	if err != nil {
		logger.Warning("Error while write state to db", err)
	}

	return nil
}

func processMessage(msg *messages.Message, chatState *database.Chat) (database.ChatState, error) {
	switch msg.MessageType {
	case messages.MESSAGE_TREATMENT_CLOSE,
		messages.MESSAGE_TREATMENT_CLOSE_ACTIVE,
		messages.MESSAGE_TREATMENT_CLOSE_DEL_LINE,
		messages.MESSAGE_TREATMENT_CLOSE_DEL_SUBS,
		messages.MESSAGE_TREATMENT_CLOSE_DEL_USER:
		_, err := HideKeyboard(msg.LineId, msg.UserId)
		if err != nil {
			logger.Warning("Get error while hide keyboardParting to line", msg.LineId, "for user", msg.UserId, "with error", err)
			return database.STATE_GREETINGS, err
		}

		return database.STATE_GREETINGS, nil
	case messages.MESSAGE_TEXT:
		keyboardMain := &[][]requests.KeyboardKey{
			{{Id: "1", Text: "Памятка сотрудника"}},
			{{Id: "2", Text: "Положение о персонале"}},
			{{Id: "3", Text: "Регламент о пожеланиях"}},
			{{Id: "9", Text: "Закрыть обращение"}},
			{{Id: "0", Text: "Перевести на специалиста"}},
		}
		keyboardParting := &[][]requests.KeyboardKey{
			{{Id: "1", Text: "Да"}, {Id: "2", Text: "Нет"}},
			{{Id: "0", Text: "Перевести на специалиста"}},
		}

		switch chatState.CurrentState {
		case database.STATE_DUMMY, database.STATE_GREETINGS:
			_, err := SendMessage(msg.LineId, msg.UserId, BOT_PHRASE_GREETING, keyboardMain)
			if err != nil {
				logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
				return database.STATE_GREETINGS, err
			}

			return database.STATE_MAIN_MENU, nil
		case database.STATE_MAIN_MENU:
			comment := BOT_PHRASE_OK
			switch strings.ToLower(msg.Text) {
			case "1", "Памятка сотрудника":
				filePath, _ := filepath.Abs(filepath.Join(cnf.FilesDir, "Памятка сотрудника.pdf"))
				_, err := SendFile(msg.LineId, msg.UserId, "Памятка сотрудника.pdf", filePath, &comment, keyboardParting)
				if err != nil {
					logger.Warning("Get error while send file to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_PARTING, nil
			case "2", "Положение о персонале":
				filePath, _ := filepath.Abs(filepath.Join(cnf.FilesDir, "Положение о персонале.pdf"))
				_, err := SendFile(msg.LineId, msg.UserId, "Положение о персонале.pdf", filePath, &comment, keyboardParting)
				if err != nil {
					logger.Warning("Get error while send file to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_PARTING, nil
			case "3", "Регламент о пожеланиях":
				filePath, _ := filepath.Abs(filepath.Join(cnf.FilesDir, "Регламент.pdf"))
				_, err := SendFile(msg.LineId, msg.UserId, "Регламент.pdf", filePath, &comment, keyboardParting)
				if err != nil {
					logger.Warning("Get error while send file to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_PARTING, nil
			case "9", "Закрыть обращение":
				_, err := CloseTreatment(msg.LineId, msg.UserId)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_GREETINGS, nil
			case "0", "Перевести на специалиста":
				_, err := RerouteTreatment(msg.LineId, msg.UserId)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_GREETINGS, nil
			default:
				_, err := SendMessage(msg.LineId, msg.UserId, BOT_PHRASE_SORRY, keyboardMain)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_MAIN_MENU, nil
			}
		case database.STATE_PARTING:
			switch strings.ToLower(msg.Text) {
			case "1", "Да":
				_, err := SendMessage(msg.LineId, msg.UserId, BOT_PHRASE_GREETING, keyboardMain)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_MAIN_MENU, nil
			case "2", "Нет":
				_, err := CloseTreatment(msg.LineId, msg.UserId)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_GREETINGS, nil
			case "0", "Перевести на специалиста":
				_, err := RerouteTreatment(msg.LineId, msg.UserId)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_GREETINGS, nil
			default:
				_, err := SendMessage(msg.LineId, msg.UserId, BOT_PHRASE_SORRY, keyboardParting)
				if err != nil {
					logger.Warning("Get error while send message to line", msg.LineId, "for user", msg.UserId, "with error", err)
					return database.STATE_GREETINGS, err
				}
				return database.STATE_PARTING, nil
			}
		}
	}

	return database.STATE_DUMMY, errors.New("I don't know hat i mus do!")
}
