package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	dem "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/events"
)

var parsers = map[string]dem.Parser{}
var headers = map[string]common.DemoHeader{}
var sockets = map[string]*websocket.Conn{}
var mtInts = map[string]int{}

func main() {
	app := fiber.New()
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws", websocket.New(websocketHandler))
	log.Fatal(app.Listen(":3000"))
}

func websocketHandler(c *websocket.Conn) {
	log.Println(c.Locals("allowed"))  // true
	log.Println(c.Query("v"))         // 1.0
	log.Println(c.Cookies("session")) // ""

	var (
		mt  int
		msg []byte
		err error
	)

	for {
		if mt, msg, err = c.ReadMessage(); err != nil {
			break
		}
		mapData := jsonToMap(msg)
		path := mapData["path"]
		mtInts[path] = mt
		sockets[path] = c
		safe(performWebsocketTask(mapData))
		log.Printf("recv: %s", msg)
	}
}

func performWebsocketTask(mapData map[string]string) error {
	path := mapData["path"]
	task := mapData["task"]
	fmt.Println(task)
	switch task {
	case "test":
		{
			return sendMessage(path, "Test")
		}
	case "parse_to_end":
		{
			return parseToEnd(path)
		}
	case "new_parser":
		{
			return newParser(path)
		}
	case "parse_header":
		{
			return parseHeader(path)
		}
	case "parse_next_frame":
		{
			return parseNextFrame(path)
		}
	case "current_frame":
		{
			return currentFrame(path)

		}
	case "frame_rate":
		{
			return frameRate(path)

		}
	case "in_game_tick":
		{
			return inGameTick(path)

		}
	case "tick_rate":
		{
			return tickRate(path)
		}
	case "close":
		{
			return closeParser(path)
		}
	default:
		{
			prefix := "register_event_handler:"
			if strings.HasPrefix(task, prefix) {
				event := strings.TrimPrefix(task, prefix)
				return registerEventHandler(path, event)
			}
			fmt.Println("Task unknown: " + task)
			return sendError(path)
		}
	}
}

func sendMessage(path string, message string) error {
	return sockets[path].WriteMessage(mtInts[path], []byte(message))
}
func sendOk(path string) error {
	return sendMessage(path, "Ok")
}

func sendError(path string) error {
	return sendMessage(path, "Error")
}

func closeParser(path string) error {
	err := sendOk(path)
	safe(parsers[path].Close())
	safe(sockets[path].Close())
	delete(mtInts, path)
	delete(parsers, path)
	delete(headers, path)
	return err
}

func currentFrame(path string) error {
	parser := parsers[path]
	tr := parser.CurrentFrame()
	trs := fmt.Sprintf("%v\n", tr)
	return sendMessage(path, trs)
}

func parseNextFrame(path string) error {
	ok, err := parsers[path].ParseNextFrame()
	safe(err)
	if ok {
		return sendMessage(path, "true")
	}
	return sendMessage(path, "false")
}

func parseToEnd(path string) error {
	safe(parsers[path].ParseToEnd())
	return sendOk(path)
}

func frameRate(path string) error {
	header := headers[path]
	fr := header.FrameRate()
	frs := fmt.Sprintf("%v\n", fr)
	return sendMessage(path, frs)
}
func inGameTick(path string) error {
	parser := parsers[path]
	tr := parser.GameState().IngameTick()
	trs := fmt.Sprintf("%v\n", tr)
	return sendMessage(path, trs)
}

func tickRate(path string) error {
	parser := parsers[path]
	tr := parser.TickRate()
	trs := fmt.Sprintf("%v\n", tr)
	return sendMessage(path, trs)
}

func newParser(path string) error {
	fmt.Printf("%s\n", path)
	f, err := os.Open(path)
	safe(err)
	parsers[path] = dem.NewParser(f)
	return sendOk(path)
}
func parseHeader(path string) error {
	p := parsers[path]
	h, err := p.ParseHeader()
	headers[path] = h
	fmt.Printf("%s\n", path)
	safe(err)
	return sendOk(path)
}

func registerEventHandler(path string, event string) error {
	switch event {
	case "PlayerHurt":
		parsers[path].RegisterEventHandler(func(e events.PlayerHurt) {
			attacker := e.Attacker
			var attackerId int
			if attacker == nil {
				attackerId = 0
			} else {
				attackerId = e.Attacker.UserID
			}

			player := e.Player
			var playerId int
			if player == nil {
				playerId = 0
			} else {
				playerId = e.Player.UserID
			}

			weapon := e.Weapon
			var weaponName string
			if weapon == nil {
				weaponName = "Unknown"
			} else {
				weaponName = e.Weapon.String()
			}

			playerHurt := fmt.Sprintf("attacker_id:%v, user_id:%v, weapon:%s", attackerId, playerId, weaponName)
			safe(sendMessage(path, playerHurt))
		})
	case "WeaponFire":
		parsers[path].RegisterEventHandler(func(e events.WeaponFire) {
			weapon := e.Weapon
			var weaponName string
			if weapon == nil {
				weaponName = "Unknown"
			} else {
				weaponName = e.Weapon.String()
			}

			shooter := e.Shooter
			var shooterId int
			if shooter == nil {
				shooterId = 0
			} else {
				shooterId = e.Shooter.UserID
			}

			weaponFire := fmt.Sprintf("user_id:%v, weapon:%s", shooterId, weaponName)
			safe(sendMessage(path, weaponFire))
		})
	default:
		fmt.Println("Event unknown: " + event)
		return sendError(path)
	}
	return sendOk(path)
}

func jsonToMap(body []byte) map[string]string {
	var result map[string]string
	safe(json.Unmarshal(body, &result))
	return result
}

func safe(err error) {
	if err != nil {
		panic(err)
	}
}
