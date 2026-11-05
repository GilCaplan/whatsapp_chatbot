# The translator add-on: dedicated en->target models (Opus-MT through
# CTranslate2) with spaCy's part-of-speech tags. Same input, same output.
# Names, places, @tags, links, numbers and emoji never reach the model: each
# becomes a token (XX0, XX1...) that it copies into the right place, and the
# original goes back there afterwards.
#
# Reads one JSON request per line on stdin ({"pack": dir, "text": ...}) and
# writes one JSON answer per line ({"text": ...} or {"error": ...}).
import json
import os
import re
import sys
import unicodedata

import ctranslate2
import sentencepiece
import spacy

nlp = spacy.load("en_core_web_sm", disable=["lemmatizer"])
loaded = {}

PROTECT = re.compile(
    r"https?://\S*[^\s.,;:!?)]|@[^\s,.:;!?]+|\d[\d.,:/-]*\d|\d"
    r"|[\U0001F000-\U0001FAFF☀-➿️‍]+"
)
# A stop only ends a sentence before a space or the end: not in "x.co", not in "well...no".
SENTENCE = re.compile(r"(?:[^\n.!?…]|[.!?…](?=[^\s.!?…]))+[.!?…]*\s*|[.!?…]+\s*|\n+")
TOKEN = re.compile(r"XX\d+")
EMOJI = re.compile(r"[\U0001F000-\U0001FAFF\u2600-\u27BF\uFE0F\u200D]+")
# Tagged as proper nouns but meant to be translated.
KEEP_TRANSLATING = {"DATE", "TIME", "CARDINAL", "ORDINAL", "PERCENT", "QUANTITY", "MONEY"}
NOT_NAMES = {"darling", "dear", "honey", "babe", "love", "mate", "sir", "madam"}


def pack(path):
    if path not in loaded:
        loaded[path] = (
            ctranslate2.Translator(os.path.join(path, "model"), device="cpu"),
            sentencepiece.SentencePieceProcessor(model_file=os.path.join(path, "sentencepiece.model")),
        )
    return loaded[path]


def name_spans(text):
    """Character spans of the proper nouns in text, neighbours joined."""
    doc = nlp(text)
    spans = []
    for i, t in enumerate(doc):
        if t.pos_ != "PROPN" or TOKEN.fullmatch(t.text) or t.ent_type_ in KEEP_TRANSLATING:
            continue
        if t.text.lower() in NOT_NAMES or (t.is_sent_start and not t.ent_type_):
            continue  # a capital at the start of a sentence proves nothing
        start, end = t.idx, t.idx + len(t.text)
        if spans and doc[i - 1].idx + len(doc[i - 1].text) + 1 >= start and text[spans[-1][1]:start].strip() == "":
            spans[-1] = (spans[-1][0], end)
        else:
            spans.append((start, end))
    return spans


# X as other alphabets write it (Latin, Cyrillic, Greek, full width) and every
# decimal digit there is.
X_LOOKALIKES = "Xx\u0425\u0445\u03a7\u03c7\uff38\uff58"
LOCAL_DIGITS = [chr(c) for c in range(sys.maxunicode + 1) if unicodedata.category(chr(c)) == "Nd"]


def protect(text):
    """Text with links, tags, numbers and emoji as tokens, and the words by token."""
    kept = {}

    def stash(m):
        token = "XX%d" % len(kept)
        kept[token] = m.group(0)
        return token

    return PROTECT.sub(stash, text), kept


def token_pattern(token):
    n = token[2:]
    digits = "".join("[%s]" % "".join(c for c in LOCAL_DIGITS if unicodedata.digit(c) == int(d)) for d in n)
    return re.compile("[%s]{1,3}%s(?!\\d)" % (X_LOOKALIKES, digits))


def restore(out, kept):
    """Puts every word back where its token landed; a lost token goes last."""
    missing = []
    for token, word in kept.items():
        # The models sometimes shorten XX1 to X1, spell it in their own
        # alphabet or write the number in their own digits.
        hit = token_pattern(token)
        if hit.search(out):
            out = hit.sub(lambda _: word, out, count=1)
        else:
            missing.append(word)
    # Every number in the text was a token, so a number here is a mangled one.
    for word in list(missing):
        m = re.search(r"\d+", out)
        if not m:
            break
        out = out[: m.start()] + word + out[m.end():]
        missing.remove(word)
    return (out.rstrip() + " " + " ".join(missing)) if missing else out


def translate_sentence(sentence, translator, sp, kept):
    core = sentence.strip()
    if not any(c.isalpha() for c in TOKEN.sub("", core)):
        return TOKEN.sub(lambda m: kept.get(m.group(0), m.group(0)), sentence)
    lead = sentence[: len(sentence) - len(sentence.lstrip())]
    trail = sentence[len(sentence.rstrip()):]
    # Trailing emoji stay out of the model altogether.
    end = ""
    while True:
        m = re.search(r"\s*(XX\d+)[.!?\u2026]*$", core)
        if not m or not EMOJI.fullmatch(kept[m.group(1)]):
            break
        end = " " + kept.pop(m.group(1)) + end
        core = core[: m.start()]
    # Each sentence numbers its own tokens from 0: the models keep small
    # numbers intact far better than large ones.
    here = {}
    for t in TOKEN.findall(core):
        if t in kept:
            here[t] = kept[t]
    spans = list(reversed(name_spans(core)))
    for start, stop in spans:
        here["N%d" % len(here)] = core[start:stop]
        core = core[:start] + "\0%d\0" % (len(here) - 1) + core[stop:]
    words, n = {}, 0

    def renumber(m):
        nonlocal n
        token = "XX%d" % n
        n += 1
        old = m.group(0)
        words[token] = here[old] if old in here else here["N" + old.strip("\0")]
        return token

    core = re.sub(r"XX\d+|\0\d+\0", renumber, core)
    if any(c.isalpha() for c in TOKEN.sub("", core)):
        result = translator.translate_batch(
            [sp.encode(core, out_type=str)], beam_size=4, max_decoding_length=256
        )
        core = sp.decode(result[0].hypotheses[0])
    out = restore(core, words)
    return lead + re.sub(r"(?<=[\u05d0-\u05ea])-\s+", "-", out) + end + trail


def translate(path, text):
    translator, sp = pack(path)
    masked, kept = protect(text)
    return "".join(translate_sentence(s, translator, sp, kept) for s in SENTENCE.findall(masked))


def main():
    for line in sys.stdin:
        try:
            req = json.loads(line)
            answer = {"text": translate(req["pack"], req["text"])}
        except Exception as err:  # the app decides what to do; keep serving
            answer = {"error": str(err)}
        sys.stdout.write(json.dumps(answer, ensure_ascii=False) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
