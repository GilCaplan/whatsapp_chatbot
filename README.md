# WhatsApp Doppelgänger (Leo)

A local, privacy-focused WhatsApp automation bot powered by **Go** and **Llama 3**. It adopts a specific persona to reply to messages and features a "Force Latch" system to handle WhatsApp's complex ID systems (LID vs. Phone JID).

## ⚡ Prerequisites

* **Go** (1.21 or higher)
* **Ollama** (running locally with `llama3`)
* **WhatsApp Mobile App** (to scan QR code)

## 🛠️ Setup

1.  **Clone & Install**
    ```bash
    git clone <your-repo-url>
    cd <your-repo-name>
    go mod tidy
    ```

2.  **Start Local AI**
    Make sure Ollama is running in a separate terminal:
    ```bash
    ollama pull llama3
    ollama serve
    ```

## ⚙️ Configuration

Open `bot.go` and edit the **CONFIG** section at the top to set your target:

* `TARGET_PHONE`: The phone number of the person you want to chat with (Format: `9175550123`, no `+`).
* `PERSONA_NAME` / `IDENTITY`: The system prompt defining the bot's personality.
* `SANDBOX_TRIGGER`: The prefix you type to control the bot (Default: `"1"`).

## 🚀 How to Run

```bash
go run bot.go