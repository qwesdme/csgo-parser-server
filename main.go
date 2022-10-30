package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws", websocket.New(websocketHandler))

	app.Post("/", rootHandler)
	log.Fatal(app.Listen(":3000"))
}

func rootHandler(c *fiber.Ctx) error {
	return performTask(c)
}

func websocketHandler(c *websocket.Conn) {
	// c.Locals is added to the *websocket.Conn
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
		performWebsocketTask(mapData)
		log.Printf("recv: %s", msg)
	}
}

func performWebsocketTask(mapData map[string]string) {
	task := mapData["task"]
	switch task {
	case "test":
		{
			fmt.Println("Test")
		}

	case "parse_to_end":
		{
			fmt.Println("ParseToEnd")
			parseToEnd(mapData["path"])
		}
	}
}

func performTask(c *fiber.Ctx) error {
	mapData := jsonToMap(c.Body())
	task := mapData["task"]
	switch task {
	case "test":
		{
			fmt.Println("Test")
		}
	case "new_parser":
		{
			fmt.Println("Parse")
			newParser(mapData["path"])

		}
	case "parse_header":
		{
			fmt.Println("ParseHeader")
			parseHeader(mapData["path"])

		}
	case "parse_next_frame":
		{
			fmt.Println("ParseNextFrame")
			return parseNextFrame(c, mapData["path"])

		}
	case "current_frame":
		{
			fmt.Println("CurrentFrame")
			return currentFrame(c, mapData["path"])

		}
	case "frame_rate":
		{
			fmt.Println("FrameRate")
			return frameRate(c, mapData["path"])

		}
	case "in_game_tick":
		{
			fmt.Println("InGameTick")
			return inGameTick(c, mapData["path"])

		}
	case "tick_rate":
		{
			fmt.Println("TickRate")
			return tickRate(c, mapData["path"])
		}
	case "close":
		{
			fmt.Println("TickRate")
			closeParser(mapData["path"])
		}
	default:
		{
			prefix := "register_event_handler:"
			if strings.HasPrefix(task, prefix) {
				event := strings.TrimPrefix(task, prefix)
				return registerEventHandler(c, mapData["path"], event)
			}
			fmt.Println("Task unknown: " + task)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
	}
	return c.SendStatus(fiber.StatusOK)
}

func closeParser(path string) {
	parser := parsers[path]
	safe(parser.Close())
	delete(parsers, path)
	delete(headers, path)
}

func currentFrame(c *fiber.Ctx, path string) error {
	parser := parsers[path]
	tr := parser.CurrentFrame()
	trs := fmt.Sprintf("%v\n", tr)
	println(trs)
	return c.SendString(trs)
}

func parseNextFrame(c *fiber.Ctx, path string) error {
	ok, err := parsers[path].ParseNextFrame()
	safe(err)
	if ok {
		return c.SendString(strconv.FormatBool(ok))
	}
	return c.SendStatus(fiber.StatusInternalServerError)
}

func parseToEnd(path string) {
	safe(parsers[path].ParseToEnd())
	safe(sockets[path].WriteMessage(mtInts[path], []byte("Done")))
}

func frameRate(c *fiber.Ctx, path string) error {
	header := headers[path]
	fr := header.FrameRate()
	frs := fmt.Sprintf("%v\n", fr)
	println(frs)
	return c.SendString(frs)
}
func inGameTick(c *fiber.Ctx, path string) error {
	parser := parsers[path]
	tr := parser.GameState().IngameTick()
	trs := fmt.Sprintf("%v\n", tr)
	println(trs)
	return c.SendString(trs)
}

func tickRate(c *fiber.Ctx, path string) error {
	parser := parsers[path]
	tr := parser.TickRate()
	trs := fmt.Sprintf("%v\n", tr)
	println(trs)
	return c.SendString(trs)
}

func newParser(path string) {
	fmt.Printf("%s\n", path)
	f, err := os.Open(path)
	safe(err)
	parsers[path] = dem.NewParser(f)
}
func parseHeader(path string) {
	p := parsers[path]
	h, err := p.ParseHeader()
	headers[path] = h
	fmt.Printf("%s\n", path)
	safe(err)
}

func registerEventHandler(c *fiber.Ctx, path string, event string) error {
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
			println(playerHurt)
			safe(sockets[path].WriteMessage(mtInts[path], []byte(playerHurt)))
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
			println(weaponFire)
			safe(sockets[path].WriteMessage(mtInts[path], []byte(weaponFire)))
		})
	default:
		fmt.Println("Event unknown: " + event)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusOK)
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
