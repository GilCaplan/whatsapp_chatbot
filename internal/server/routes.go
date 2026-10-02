package server

import "net/http"

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// System
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/system/ports", s.handlePorts)
	mux.HandleFunc("POST /api/system/port", s.handleChangePort)
	mux.HandleFunc("POST /api/system/quit", s.handleQuit)
	mux.HandleFunc("POST /api/system/open-data-dir", s.handleOpenDataDir)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	// Settings & LLM
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("PUT /api/settings/secrets", s.handlePutSecrets)
	mux.HandleFunc("POST /api/llm/test", s.handleLLMTest)
	mux.HandleFunc("GET /api/llm/models", s.handleLLMModels)
	mux.HandleFunc("GET /api/ollama/status", s.handleOllamaStatus)
	mux.HandleFunc("POST /api/ollama/pull", s.handleOllamaPull)

	// Reply behaviour
	mux.HandleFunc("GET /api/behavior/presets", s.handleBehaviorPresets)
	mux.HandleFunc("POST /api/behavior/sample", s.handleBehaviorSample)

	// WhatsApp
	mux.HandleFunc("GET /api/wa/status", s.handleWAStatus)
	mux.HandleFunc("POST /api/wa/pair", s.handleWAPair)
	mux.HandleFunc("GET /api/wa/qr.png", s.handleWAQR)
	mux.HandleFunc("POST /api/wa/reconnect", s.handleWAReconnect)
	mux.HandleFunc("POST /api/wa/disconnect", s.handleWADisconnect)
	mux.HandleFunc("POST /api/wa/logout", s.handleWALogout)
	mux.HandleFunc("GET /api/wa/chats", s.handleWAChats)
	mux.HandleFunc("POST /api/wa/chats/refresh", s.handleWARefresh)
	mux.HandleFunc("GET /api/wa/avatar", s.handleWAAvatar)

	// Chat assignments
	mux.HandleFunc("GET /api/chats", s.handleListChats)
	mux.HandleFunc("POST /api/chats", s.handleCreateChat)
	mux.HandleFunc("PATCH /api/chats/{key}", s.handlePatchChat)
	mux.HandleFunc("DELETE /api/chats/{key}", s.handleDeleteChat)
	mux.HandleFunc("GET /api/chats/{key}/behavior", s.handleChatBehavior)
	mux.HandleFunc("GET /api/chats/{key}/people", s.handleChatPeople)
	mux.HandleFunc("POST /api/chats/{key}/goal/reset", s.handleGoalReset)
	mux.HandleFunc("POST /api/chats/{key}/initiate", s.handleInitiate)
	mux.HandleFunc("GET /api/chats/{key}/history", s.handleChatHistory)
	mux.HandleFunc("DELETE /api/chats/{key}/history", s.handleClearChatHistory)
	mux.HandleFunc("POST /api/chats/{key}/send", s.handleChatSend)
	mux.HandleFunc("POST /api/chats/{key}/simulate", s.handleChatSimulate)

	// Personas
	mux.HandleFunc("GET /api/personas", s.handleListPersonas)
	mux.HandleFunc("POST /api/personas", s.handleCreatePersona)
	mux.HandleFunc("POST /api/personas/draft", s.handleDraftPersona)
	mux.HandleFunc("GET /api/personas/{id}", s.handleGetPersona)
	mux.HandleFunc("PUT /api/personas/{id}", s.handlePutPersona)
	mux.HandleFunc("DELETE /api/personas/{id}", s.handleDeletePersona)
	mux.HandleFunc("POST /api/personas/{id}/duplicate", s.handleDuplicatePersona)
	mux.HandleFunc("POST /api/personas/{id}/reset", s.handleResetPersona)
	mux.HandleFunc("PUT /api/personas/{id}/avatar", s.handleUploadAvatar)
	mux.HandleFunc("DELETE /api/personas/{id}/avatar", s.handleDeleteAvatar)
	mux.HandleFunc("GET /api/personas/{id}/avatar", s.handleGetAvatar)
	mux.HandleFunc("GET /api/personas/{id}/prompt-preview", s.handlePromptPreview)
	mux.HandleFunc("POST /api/personas/{id}/expression-preview", s.handleExpressionPreview)

	// Playground
	mux.HandleFunc("POST /api/playground", s.handlePlaygroundStart)
	mux.HandleFunc("POST /api/playground/{id}/messages", s.handlePlaygroundSend)
	mux.HandleFunc("DELETE /api/playground/{id}", s.handlePlaygroundEnd)

	// Approvals
	mux.HandleFunc("GET /api/approvals", s.handleListApprovals)
	mux.HandleFunc("POST /api/approvals/{id}/approve", s.handleApprove)
	mux.HandleFunc("POST /api/approvals/{id}/regenerate", s.handleRegenerate)
	mux.HandleFunc("DELETE /api/approvals/{id}", s.handleDiscard)

	// Activity
	mux.HandleFunc("GET /api/activity", s.handleActivity)
	mux.HandleFunc("DELETE /api/activity", s.handleClearActivity)

	// ── Wave 3 (frozen routes; handlers live in one file per feature) ──

	// Memory of people (handlers_memories.go)
	mux.HandleFunc("GET /api/chats/{key}/memories", s.handleListMemories)
	mux.HandleFunc("POST /api/chats/{key}/memories", s.handleAddMemory)
	mux.HandleFunc("POST /api/chats/{key}/memories/extract", s.handleExtractMemories)
	mux.HandleFunc("PATCH /api/chats/{key}/memories/{id}", s.handlePatchMemory)
	mux.HandleFunc("DELETE /api/chats/{key}/memories/{id}", s.handleDeleteMemory)
	mux.HandleFunc("DELETE /api/chats/{key}/memories", s.handleClearMemories)

	// Hand-off and reveal (handlers_handoff.go, handlers_reveal.go)
	mux.HandleFunc("POST /api/chats/{key}/handoff/resume", s.handleResumeHandoff)
	mux.HandleFunc("POST /api/chats/{key}/reveal", s.handleReveal)

	// Daily recap (handlers_recaps.go)
	mux.HandleFunc("GET /api/recaps", s.handleListRecaps)
	mux.HandleFunc("POST /api/recaps/generate", s.handleGenerateRecaps)

	// Missions (handlers_missions.go)
	mux.HandleFunc("GET /api/missions", s.handleMissions)
	mux.HandleFunc("POST /api/missions/start", s.handleStartMission)
	mux.HandleFunc("POST /api/missions/{chatKey}/abandon", s.handleAbandonMission)

	// Clone yourself (handlers_clone.go)
	mux.HandleFunc("GET /api/clone/samples", s.handleCloneSamples)
	mux.HandleFunc("DELETE /api/clone/samples", s.handleDeleteCloneSamples)
	mux.HandleFunc("POST /api/clone/draft", s.handleCloneDraft)

	// Notifications (handlers_notify.go)
	mux.HandleFunc("POST /api/system/notify-test", s.handleNotifyTest)

	// Unknown API paths get a JSON 404; everything else is the UI.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "No such endpoint")
	})
	mux.HandleFunc("/", s.handleStatic)
	return mux
}
