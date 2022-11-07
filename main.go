package main

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	dem "github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/events"
	dispatch "github.com/markus-wa/godispatch"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var parsers = map[string]dem.Parser{}
var headers = map[string]common.DemoHeader{}
var sockets = map[string]*websocket.Conn{}
var mtInts = map[string]int{}

var markedFrames = map[string]map[int]bool{}

var app *fiber.App

func main() {
	dem.DefaultParserConfig.IgnoreErrBombsiteIndexNotFound = true

	app = fiber.New()
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
	}
}

func performWebsocketTask(mapData map[string]string) error {
	path := mapData["path"]
	task := mapData["task"]
	switch task {
	case "test":
		{
			return sendMessage(path, "Test")
		}
	case "parse_to_end":
		{
			return parseToEnd(path)
		}
	case "parse_to_end_with_marked_frames":
		{
			return parseToEndWithMarkedFrames(path)
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
	case "shutdown":
		{
			safe(sendOk(path))
			return app.Shutdown()
		}
	default:
		{
			prefixRegister := "register_event_handler:"
			if strings.HasPrefix(task, prefixRegister) {
				event := strings.TrimPrefix(task, prefixRegister)
				return registerEventHandler(path, event)
			}

			prefixUnregister := "unregister_event_handler:"
			if strings.HasPrefix(task, prefixUnregister) {
				handlerId := strings.TrimPrefix(task, prefixUnregister)
				return unregisterEventHandler(path, handlerId)
			}

			prefixMarkFrames := "mark_frames:"
			if strings.HasPrefix(task, prefixMarkFrames) {
				frames := strings.TrimPrefix(task, prefixMarkFrames)
				return markFrames(path, frames)
			}
			fmt.Println("Task unknown: " + task)
			return sendError(path)
		}
	}
}

func markFrames(path string, framesStr string) error {
	frameStrSplit := strings.Split(framesStr, "\t")
	frames := map[int]bool{}
	for _, frameStr := range frameStrSplit {
		frame, err := strconv.Atoi(frameStr)
		safe(err)
		frames[frame] = true
	}
	markedFrames[path] = frames
	return sendOk(path)
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
	fmt.Printf("Done: %s\n", path)
	return err
}

func currentFrame(path string) error {
	parser := parsers[path]
	tr := parser.CurrentFrame()
	trs := fmt.Sprintf("%v", tr)
	return sendMessage(path, trs)
}

func playingStr(path string) string {
	parser := parsers[path]
	playing := parser.GameState().Participants().Playing()

	playingStr := ""
	for i, player := range playing {
		if i > 0 {
			playingStr += "\n"
		}
		playingStr += fmt.Sprintf("%v,%v,%v,%s,", player.UserID, player.FlashDuration, player.SteamID64, player.Name)
		playingStr += fmt.Sprintf("%v,%v,%v,", player.LastAlivePosition.X, player.LastAlivePosition.Y, player.LastAlivePosition.Z)
		playerVelocity := player.Velocity()
		playingStr += fmt.Sprintf("%v,%v,%v,", playerVelocity.X, playerVelocity.Y, playerVelocity.Z)
		playingStr += fmt.Sprintf("%s,", activeWeapon(player))
		playingStr += fmt.Sprintf("%v,%v,", player.ViewDirectionX(), player.ViewDirectionY())
		playingStr += fmt.Sprintf("%v,%v", player.IsDucking(), player.Health())
	}
	return playingStr
}

func activeWeapon(player *common.Player) string {
	activeWeapon := player.ActiveWeapon()
	if activeWeapon == nil {
		return "Unknown"
	} else {
		return activeWeapon.String()
	}
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
func parseToEndWithMarkedFrames(path string) error {
	var err error
	p := parsers[path]
	now := time.Now()
	for ok := true; ok; ok, err = p.ParseNextFrame() {
		safe(err)
		currentFrame := p.CurrentFrame()
		if markedFrames[path][currentFrame] {
			markedFrame := fmt.Sprintf("%v,%s", currentFrame, playingStr(path))
			safe(sendMessage(path, markedFrame))
		}
	}
	fmt.Printf("Second pass done in %v\n", time.Since(now))
	return sendOk(path)
}

func frameRate(path string) error {
	header := headers[path]
	fr := header.FrameRate()
	frs := fmt.Sprintf("%v", fr)
	return sendMessage(path, frs)
}
func inGameTick(path string) error {
	parser := parsers[path]
	tr := parser.GameState().IngameTick()
	trs := fmt.Sprintf("%v", tr)
	return sendMessage(path, trs)
}

func tickRate(path string) error {
	parser := parsers[path]
	tr := parser.TickRate()
	trs := fmt.Sprintf("%v", tr)
	return sendMessage(path, trs)
}

func newParser(path string) error {
	fmt.Printf("New parser: %s\n", path)
	f, err := os.Open(path)
	safe(err)
	parsers[path] = dem.NewParser(f)
	return sendOk(path)
}
func parseHeader(path string) error {
	p := parsers[path]
	h, err := p.ParseHeader()
	headers[path] = h
	safe(err)
	return sendOk(path)
}

func registerEventHandler(path string, event string) error {
	switch event {
	case "PlayerHurt":
		{
			parsers[path].RegisterEventHandler(func(e events.PlayerHurt) {
				attackerId := userID(e.Attacker)
				playerId := userID(e.Player)
				weaponName := weaponName(e.Weapon)
				frame := parsers[path].CurrentFrame()

				playerHurt := fmt.Sprintf("event:PlayerHurt, attacker_id:%v, player_id:%v, weapon:%s, frame:%v", attackerId, playerId, weaponName, frame)
				safe(sendMessage(path, playerHurt))
			})
		}
	case "WeaponFire":
		{
			parsers[path].RegisterEventHandler(func(e events.WeaponFire) {
				weaponName := weaponName(e.Weapon)
				shooterId := userID(e.Shooter)
				frame := parsers[path].CurrentFrame()
				weaponFire := fmt.Sprintf("event:WeaponFire, player_id:%v, weapon:%s, frame:%v", shooterId, weaponName, frame)
				safe(sendMessage(path, weaponFire))
			})
		}
	default:
		fmt.Println("Event unknown: " + event)
		return sendError(path)
	}
	fmt.Printf("Registered:%s\n", event)
	return sendOk(path)
}

func weaponName(weapon *common.Equipment) string {
	if weapon == nil {
		return "Unknown"
	}
	return weapon.String()
}

func userID(player *common.Player) int {
	if player == nil {
		return 0
	}
	return player.UserID
}

func unregisterEventHandler(path string, handlerId string) error {
	id, err := strconv.Atoi(handlerId)
	safe(err)
	var hid dispatch.HandlerIdentifier
	hid = &id
	parsers[path].UnregisterEventHandler(hid)
	return sendOk(path)
}

func jsonToMap(body []byte) map[string]string {
	var result map[string]string
	safe(json.Unmarshal(body, &result))
	return result
}

func safe(err error) {
	if err != nil {
		if err == dem.ErrUnexpectedEndOfDemo {
			fmt.Println("Warning: ErrUnexpectedEndOfDemo")
		} else {
			panic(err)
		}
	}
}
