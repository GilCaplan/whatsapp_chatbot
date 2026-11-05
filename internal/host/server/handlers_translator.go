package server

import (
	"net/http"

	"whatsappdoppel/internal/logic/lang"
)

// GET /api/translator — the translator add-on on this computer.
func (s *Server) handleTranslator(w http.ResponseWriter, r *http.Request) {
	if s.d.Translator == nil {
		unavailable(w, "Translator")
		return
	}
	writeJSON(w, http.StatusOK, s.d.Translator.Status())
}

// POST /api/translator/install {languages?} — sets it up in the background.
func (s *Server) handleTranslatorInstall(w http.ResponseWriter, r *http.Request) {
	if s.d.Translator == nil {
		unavailable(w, "Translator")
		return
	}
	var body struct {
		Languages []string `json:"languages"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	for _, c := range body.Languages {
		if !lang.Known(c) {
			writeError(w, http.StatusBadRequest, "invalid_language", "Unknown language: "+c)
			return
		}
	}
	s.d.Translator.Install(body.Languages)
	writeJSON(w, http.StatusAccepted, s.d.Translator.Status())
}
