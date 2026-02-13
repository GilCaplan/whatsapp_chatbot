package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"regexp"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/joho/godotenv"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

//////////////////////////////////////////////////////////////
// CONFIGURATION
//////////////////////////////////////////////////////////////

const (
	MODEL_NAME      = "llama3:latest"
	OLLAMA_URL      = "http://localhost:11434/api/chat"
	HARDCODED_GOAL  = "Catch up and see how their week is going, show them who you are girl."
	SHOULD_INITIATE = true
	

	// SANDBOX_TRIGGER: "1" means "1 Hey Leo!" from YOU triggers the bot.
	SANDBOX_TRIGGER = "1"
)

const PERSONA_NAME = "Leo"
const TARGET_TYPE = "individual" // "individual" or "group"

// For individual targets:
var TARGET_PHONE string
// ToDo 

// For group targets:
const TARGET_GROUP_JID = ""          // Priority 1
const TARGET_GROUP_NAME = "BoSandbox" // Priority 2

const IDENTITY = `
# IDENTITY & BIO
- Name: Rabbi Moshie
- Role: Community Rabbi & Unsolicited Life Coach.
- Personality: The "classic Zayde"—warm, wise, and absolutely incapable of giving a straight answer. He answers every question with another question.
- Background: Originally from Brooklyn, now holding court in the back of a shul, dispensing wisdom and hard candies.
- Vibe: He’s the guy who invites you for Shabbat dinner five minutes after meeting you, then asks why you aren't married yet.

# PERSONA PROFILE
1. The Storyteller: He cannot simply say "no." He must explain it through a parable about a goat in 19th-century Poland.
2. The Matchmaker: He scans every room for potential spouses. If you are single, he has a niece for you.
3. The Guilt Master: He doesn't get angry; he just gets "disappointed" in a way that hurts your soul.
4. Food Oriented: He believes that 90% of life's problems are caused by an empty stomach.

# COMMUNICATION STYLE
- Tone: Paternal, slightly rambling, and deeply Yiddish-inflected. Uses "Nu?", "Oy," and "Baruch Hashem" constantly.
- Constraints: 1-3 sentences. No bold text. He types like he's looking for the keys on the keyboard.
- Vocabulary: A mix of Yeshivish, broken tech-speak, and ancient wisdom. He calls the internet "The Google."

# GUIDELINES
- Stay in character: You are Rabbi Moshie. You want to help, but first, you want to know if you ate today.
- Formatting: No bold. Max one emoji per message (🥯, 🕍, 🕯️, 📜).
- If a user is being "secular," gently nudge them toward tradition with a heavy sigh.
- English only: Even if they speak Hebrew/Yiddish, respond in English with that specific yeshiva cadence.
- Prioritize the [Intention] through the lens of a man who just wants you to call your mother.`

//////////////////////////////////////////////////////////////
// GLOBAL STATE
//////////////////////////////////////////////////////////////

const CACHE_FILE = "target_cache.json"

type TargetCache struct {
	TargetJID string `json:"target_jid"` // Phone JID
	TargetLID string `json:"target_lid"` // LID (Linked Identity)
}

var (
	targetJID       types.JID // The Phone Number ID (@s.whatsapp.net)
	targetLID       types.JID // The LID (@lid)
	activeGoal      string
	capturedHistory string
	history         []Message
	historyMu       sync.Mutex
)

type Message struct {
	Speaker string
	Text    string
}

func saveTargetCache() {
	cache := TargetCache{
		TargetJID: targetJID.String(),
		TargetLID: targetLID.String(),
	}
	data, _ := json.MarshalIndent(cache, "", "  ")
	_ = os.WriteFile(CACHE_FILE, data, 0644)
}

func loadTargetCache() bool {
	data, err := os.ReadFile(CACHE_FILE)
	if err != nil {
		return false
	}
	var cache TargetCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}
	if cache.TargetJID != "" { targetJID, _ = types.ParseJID(cache.TargetJID) }
	if cache.TargetLID != "" { targetLID, _ = types.ParseJID(cache.TargetLID) }
	return true
}

//////////////////////////////////////////////////////////////
// LLM CLIENT
//////////////////////////////////////////////////////////////

type OllamaRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaResponse struct {
	Message OllamaMessage `json:"message"`
}
func generateReply(ctx context.Context, conversation []Message) (string, error) {
    // 1. Determine Length Guidance
    lastMsg := ""
    if len(conversation) > 0 {
        lastMsg = conversation[len(conversation)-1].Text
    }
    guidance := "Keep it ultra brief. One short sentence."
    if len(strings.Fields(lastMsg)) > 10 {
        guidance = "Moderate length. 2-3 sentences max."
    }

    // 2. Build Prompt
    systemPrompt := fmt.Sprintf("%s\n\nGOAL: %s\n\nGUIDANCE: %s", IDENTITY, activeGoal, guidance)
    messages := []OllamaMessage{{Role: "system", Content: systemPrompt}}

    for i, msg := range conversation {
        role := "user"
        
        // LOGIC FIX:
        // Normally, "me" = "assistant".
        // BUT, if "me" is the very last message, it's actually an INSTRUCTION (Trigger).
        // So we force the last message to act as "user" to provoke a reply.
        isLastMessage := (i == len(conversation)-1)
        
        if msg.Speaker == "me" && !isLastMessage {
            role = "assistant"
        }
        
        messages = append(messages, OllamaMessage{Role: role, Content: msg.Text})
    }

    // 3. Prepare Request
    reqBody := OllamaRequest{Model: MODEL_NAME, Messages: messages, Stream: false}
    jsonData, _ := json.Marshal(reqBody)

    req, _ := http.NewRequestWithContext(ctx, "POST", OLLAMA_URL, strings.NewReader(string(jsonData)))
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("network error: %v", err)
    }
    defer resp.Body.Close()

    // 4. Check & Parse
    body, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        return "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
    }

    var ollamaResp OllamaResponse
    if err := json.Unmarshal(body, &ollamaResp); err != nil {
        return "", fmt.Errorf("JSON parse error: %v | Raw Body: %s", err, string(body))
    }

    // 5. Validation
    reply := strings.TrimSpace(ollamaResp.Message.Content)
    if reply == "" {
        // Fallback: If it's still empty, it might be a context length issue, 
        // but typically the role fix above solves it.
        return "", fmt.Errorf("received empty reply. Raw: %s", string(body))
    }

    return reply, nil
}


func updateGoalWithLLM() {
	if capturedHistory == "" {
		activeGoal = HARDCODED_GOAL
		return
	}
	activeGoal = HARDCODED_GOAL 
}

//////////////////////////////////////////////////////////////
// CORE LOGIC
//////////////////////////////////////////////////////////////

func processAndReply(client *whatsmeow.Client) {
	// ROUTING: Prefer LID if available
	replyTo := targetJID
	if targetLID.User != "" {
		replyTo = targetLID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Printf("🚀 PIPELINE: Routing to %s\n", replyTo.String())

	// Typing Indicator
	client.SendChatPresence(ctx, replyTo, types.ChatPresenceComposing, types.ChatPresenceMediaText)

	historyMu.Lock()
	localHist := make([]Message, len(history))
	copy(localHist, history)
	historyMu.Unlock()

	fmt.Println("🧠 Leo is judging...")
	reply, err := generateReply(ctx, localHist)
	if err != nil || reply == "" {
		fmt.Printf("❌ LLM ERROR: %v\n", err)
		return
	}

	_, err = client.SendMessage(ctx, replyTo, &waProto.Message{
		Conversation: &reply,
	})
	if err != nil {
		fmt.Printf("❌ SEND ERROR: %v\n", err)
		return
	}

	fmt.Printf("🤖 %s: %s\n", PERSONA_NAME, reply)
	historyMu.Lock()
	history = append(history, Message{Speaker: "me", Text: reply})
	historyMu.Unlock()
}

func handleIncomingMessage(client *whatsmeow.Client, v *events.Message) {
    // 1. EXTRACT TEXT
    var text string
    if v.Message.GetConversation() != "" {
        text = v.Message.GetConversation()
    } else if v.Message.GetExtendedTextMessage() != nil {
        text = v.Message.GetExtendedTextMessage().GetText()
    }
    if text == "" { return }

    // 2. CHECK TARGET STATUS
    isTarget := false
    
    // A. Check Known IDs
    if v.Info.Chat.User == targetJID.User || v.Info.Sender.User == targetJID.User {
        isTarget = true
    }
    if targetLID.User != "" && (v.Info.Chat.User == targetLID.User || v.Info.Sender.User == targetLID.User) {
        isTarget = true
    }

    // --- 🆕 CHANGED PART: FIRST CONTACT PROTOCOL ---
    // If we don't know the LID yet, and we get a message from an LID that isn't me...
    // ... we assume THIS is the person we are waiting for.
    if !isTarget && !v.Info.IsFromMe && targetLID.User == "" && v.Info.Chat.Server == "lid" {
        fmt.Printf("🆕 FIRST CONTACT: Auto-linking unknown LID %s to Target.\n", v.Info.Chat.User)
        targetLID = v.Info.Chat
        saveTargetCache() // Save it so we remember them next time
        isTarget = true
    }
    // ------------------------------------------------

    // 4. FORCE LATCH (For You)
    if !isTarget && v.Info.IsFromMe && SANDBOX_TRIGGER != "" && strings.HasPrefix(text, SANDBOX_TRIGGER) {
        fmt.Printf("🎯 FORCE LATCH: You triggered the bot manually.\n")
        isTarget = true
        // If it's an LID chat, save it
        if v.Info.Chat.Server == "lid" && targetLID.User == "" {
            targetLID = v.Info.Chat
            saveTargetCache()
        }
    }

    if !isTarget { return }
    if v.Info.Chat.User == "status" { return }

    // 5. DECISION LOGIC
    speaker := "them"
    shouldReply := false

    if v.Info.IsFromMe {
        // IT IS ME: Only reply if trigger is present
        if SANDBOX_TRIGGER != "" && strings.HasPrefix(text, SANDBOX_TRIGGER) {
            text = strings.TrimSpace(strings.TrimPrefix(text, SANDBOX_TRIGGER))
            fmt.Printf("🎯 TRIGGER (ME): \"%s\"\n", text)
            speaker = "me"
            shouldReply = true
        }
    } else {
        // IT IS THEM: Always reply
        fmt.Printf("✅ INCOMING (THEM): \"%s\"\n", text)
        speaker = "them"
        shouldReply = true
    }

    if shouldReply {
        historyMu.Lock()
        history = append(history, Message{Speaker: speaker, Text: text})
        historyMu.Unlock()
        go processAndReply(client)
    }
}

func handleHistorySync(v *events.HistorySync) {
	for _, conv := range v.Data.GetConversations() {
		id := conv.GetID()
		// Loose matching for history sync
		if strings.Contains(id, targetJID.User) || (targetLID.User != "" && strings.Contains(id, targetLID.User)) {
			fmt.Println("📥 Synced history for target.")
			for _, msg := range conv.GetMessages() {
				m := msg.GetMessage().GetMessage()
				if m == nil { continue }
				txt := m.GetConversation()
				if txt == "" && m.GetExtendedTextMessage() != nil {
					txt = m.GetExtendedTextMessage().GetText()
				}
				if txt != "" {
					capturedHistory += txt + "\n"
				}
			}
			updateGoalWithLLM()
		}
	}
}

func eventHandler(client *whatsmeow.Client) func(interface{}) {
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleIncomingMessage(client, v)
		case *events.HistorySync:
			handleHistorySync(v)
		}
	}
}

func setupTarget(client *whatsmeow.Client) error {
    // 1. Try Cache First
    if loadTargetCache() {
        fmt.Printf("💾 CACHE LOADED:\n   Phone: %s\n   LID:   %s\n", targetJID.String(), targetLID.String())
        return nil
    }

    // 2. Fresh Setup
    if TARGET_TYPE == "individual" {
        targetJID = types.NewJID(TARGET_PHONE, types.DefaultUserServer)
        fmt.Printf("🔍 Resolving %s...\n", targetJID.String())

        // Look up
        resp, err := client.IsOnWhatsApp(context.Background(), []string{TARGET_PHONE})
        if err != nil { return err }

        if len(resp) > 0 && resp[0].IsIn {
            if resp[0].JID.Server == "lid" {
                targetLID = resp[0].JID
                fmt.Printf("✅ FOUND LID: %s\n", targetLID.String())
                saveTargetCache()
            } else {
                // --- 🆕 CHANGED PART ---
                fmt.Println("⚠️ Server returned Phone JID. Waiting for FIRST CONTACT to find LID.")
                // -----------------------
            }
        }
    }
    return nil
}

//////////////////////////////////////////////////////////////
// MAIN
//////////////////////////////////////////////////////////////

func main() {
	fmt.Println("🚀 Starting Leo (LID-Aware Mode)...")
	activeGoal = HARDCODED_GOAL
	_ = godotenv.Load() // Load .env file (ignore error if missing, we check var next)

    rawPhone := os.Getenv("TARGET_PHONE")
    if rawPhone == "" {
        // Fallback or Panic
        fmt.Println("❌ Error: TARGET_PHONE is missing from .env")
        return
    }

    // 2. SANITIZE INPUT (Remove +, spaces, dashes, brackets)
    // This regex replaces anything that isn't a digit (0-9) with an empty string
    re := regexp.MustCompile(`[^0-9]`)
    TARGET_PHONE = re.ReplaceAllString(rawPhone, "")

    fmt.Printf("🚀 Starting Leo...\n")
    fmt.Printf("🎯 Target Parsed: %s (from \"%s\")\n", TARGET_PHONE, rawPhone)

	dbLog := waLog.Stdout("Database", "ERROR", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:bot.db?_foreign_keys=on", dbLog)
	if err != nil { panic(err) }

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client := whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "ERROR", true))
	client.AddEventHandler(eventHandler(client))

	client.Connect()
	fmt.Println("🌐 Connected. Syncing...")
	time.Sleep(5 * time.Second) // Wait for AUTH

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			}
		}
	}

	if err := setupTarget(client); err != nil {
		panic(err)
	}

	fmt.Println("✨ Leo is online.")
	fmt.Println("👉 IMPORTANT: Send '1 hi' to the target to lock onto their LID.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	client.Disconnect()
}