# The translator add-on: dedicated en->target models (Argos packs, run by
# CTranslate2) with spaCy's part-of-speech tags. Same input, same output.
# Names, places, @tags, links, numbers and emoji never reach the model: each
# becomes a token (XX0, XX1...) that it copies into the right place, and the
# original goes back there afterwards.
#
# Reads one JSON request per line on stdin ({"pack": dir, "text": ...}) and
# writes one JSON answer per line ({"text": ...} or {"error": ...}).
import os
import re
import sys
import unicodedata

from pydantic import BaseModel, ConfigDict, Field

nlp = None
loaded = {}


class Request(BaseModel):
    # hide_input: a bad request's error must not carry someone's message.
    model_config = ConfigDict(extra="forbid", strict=True, hide_input_in_errors=True)
    pack: str = Field(min_length=1)
    text: str


class Answer(BaseModel):
    text: str | None = None
    error: str | None = None

PROTECT = re.compile(
    r"https?://\S*[^\s.,;:!?)]|@[^\s,.:;!?]+|\d[\d.,:/-]*\d|\d"
    r"|[\U0001F000-\U0001FAFF☀-➿️‍]+"
)
# A stop only ends a sentence before a space or the end: not in "x.co", not in "well...no".
SENTENCE = re.compile(r"(?:[^\n.!?…]|[.!?…]+(?=[^\s.!?…]))+[.!?…]*\s*|[.!?…]+\s*|\n+")
TOKEN = re.compile(r"XX\d+")
EMOJI = re.compile(r"[\U0001F000-\U0001FAFF\u2600-\u27BF\uFE0F\u200D]+")
# Tagged as proper nouns but meant to be translated.
KEEP_TRANSLATING = {"DATE", "TIME", "CARDINAL", "ORDINAL", "PERCENT", "QUANTITY", "MONEY"}
NOT_NAMES = {"darling", "dear", "honey", "babe", "love", "mate", "sir", "madam"}


def pack(path):
    import ctranslate2
    import sentencepiece

    if path not in loaded:
        loaded[path] = (
            ctranslate2.Translator(os.path.join(path, "model"), device="cpu"),
            sentencepiece.SentencePieceProcessor(model_file=os.path.join(path, "sentencepiece.model")),
        )
    return loaded[path]


def name_spans(text):
    """Character spans of the proper nouns in text, neighbours joined."""
    global nlp
    if nlp is None:
        import spacy

        nlp = spacy.load("en_core_web_sm", disable=["lemmatizer"])
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
# Every number in the text was a token, so a number in the output is a mangled
# one. The Arabic models also read XX as a Roman numeral and write "twenty",
# which only counts when the sentence didn't say twenty itself.
NUMBER = re.compile(r"\d+")
MANGLED = re.compile("\\d+|\\b(?:\u0627\u0644)?\u0639\u0634\u0631(?:\u0648\u0646|\u064a\u0646)\\b")


def protect(text):
    """Text with links, tags, numbers and emoji as tokens, and the words by token."""
    kept = {}

    def stash(m):
        token = "XX%d" % len(kept)
        kept[token] = m.group(0)
        return token

    return PROTECT.sub(stash, text), kept


def unmask(text, kept):
    """Text with its tokens turned back into the words they hide."""
    return TOKEN.sub(lambda m: kept.get(m.group(0), m.group(0)), text)


def token_pattern(token):
    n = token[2:]
    digits = "".join("[%s]" % "".join(c for c in LOCAL_DIGITS if unicodedata.digit(c) == int(d)) for d in n)
    return re.compile("[%s]{1,3}%s(?!\\d)" % (X_LOOKALIKES, digits))


def restore(out, kept, source):
    """Puts every word back where its token landed; a lost token goes last."""
    # Each word waits as one private-use character until the end, so the search
    # for a token never lands inside a link or time that is already back.
    marks = {chr(0xE000 + i): word for i, word in enumerate(kept.values())}
    missing = []
    for token, mark in zip(kept, marks):
        # The models sometimes shorten XX1 to X1, spell it in their own
        # alphabet or write the number in their own digits, and they can write
        # it twice: every copy goes, as a token left in the text sinks the reply.
        hit = token_pattern(token)
        if hit.search(out):
            out = hit.sub(mark, out)
        else:
            missing.append(mark)
    mangled = NUMBER if "twent" in source.lower() else MANGLED
    for mark in list(missing):
        m = mangled.search(out)
        if not m:
            break
        out = out[: m.start()] + mark + out[m.end():]
        missing.remove(mark)
    if missing:
        out = out.rstrip() + " " + " ".join(missing)
    return out.translate(str.maketrans(marks))


def translate_sentence(sentence, translator, sp, kept):
    core = sentence.strip()
    if not any(c.isalpha() for c in TOKEN.sub("", core)):
        return unmask(sentence, kept)
    lead = sentence[: len(sentence) - len(sentence.lstrip())]
    trail = sentence[len(sentence.rstrip()):]
    # Trailing emoji stay out of the model altogether.
    end = ""
    while True:
        m = re.search(r"\s*(XX\d+)([.!?\u2026]*)$", core)
        if not m or not EMOJI.fullmatch(kept[m.group(1)]):
            break
        end = " " + kept.pop(m.group(1)) + m.group(2) + end
        core = core[: m.start()]
    # Each sentence numbers its own tokens from 0: the models keep small
    # numbers intact far better than large ones.
    here = {}
    for t in TOKEN.findall(core):
        if t in kept:
            here[t] = kept[t]
    spans = list(reversed(name_spans(core)))
    for start, stop in spans:
        # A name can swallow a token stuck to it, as in "iPhone15" or "Dana🎉".
        here["N%d" % len(here)] = unmask(core[start:stop], kept)
        core = core[:start] + "\0%d\0" % (len(here) - 1) + core[stop:]
    words, n = {}, 0

    def renumber(m):
        nonlocal n
        token = "XX%d" % n
        n += 1
        old = m.group(0)
        words[token] = here[old] if old in here else here["N" + old.strip("\0")]
        return token

    core = source = re.sub(r"XX\d+|\0\d+\0", renumber, core)
    if any(c.isalpha() for c in TOKEN.sub("", core)):
        result = translator.translate_batch(
            [sp.encode(core, out_type=str)], beam_size=4, max_decoding_length=256, disable_unk=True
        )
        # Not sp.decode: it keeps the word marker on any piece the pack's
        # tokenizer doesn't know, which is most of them in newer packs. This
        # join would send a <unk> as it stands, so the search can't pick one.
        core = "".join(result[0].hypotheses[0]).replace("\u2581", " ").strip()
    out = restore(core, words, source)
    return lead + re.sub(r"(?<=[\u05d0-\u05ea])-\s+", "-", out) + end + trail


def translate(path, text):
    translator, sp = pack(path)
    masked, kept = protect(text)
    return "".join(translate_sentence(s, translator, sp, kept) for s in SENTENCE.findall(masked))


def answer(line):
    try:
        req = Request.model_validate_json(line)
        return Answer(text=translate(req.pack, req.text))
    except Exception as err:  # noqa: BLE001 - the app decides what to do; keep serving
        # Some errors have no text, and an empty error reads as a success.
        return Answer(error=str(err) or type(err).__name__)


def main():
    # The app speaks UTF-8; Windows would otherwise read the pipe in its own code page.
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    for line in sys.stdin:
        sys.stdout.write(answer(line).model_dump_json(exclude_none=True) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
