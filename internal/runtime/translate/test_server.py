# The add-on's text handling and its line protocol. These run without Python's
# heavy libraries or any model: the model is a fake that "translates" by a table.
import json
import os
import subprocess
import sys
from types import SimpleNamespace

import pytest

import server


class FakeSP:
    def encode(self, text, out_type=str):
        return [text]


class FakeTranslator:
    def __init__(self, reply):
        self.reply = reply
        self.options = {}
        self.seen = None

    def translate_batch(self, batch, **options):
        self.seen, self.options = batch[0][0], options
        return [SimpleNamespace(hypotheses=[list(self.reply)])]


def run(sentence, reply, monkeypatch, names=(), keep=()):
    monkeypatch.setattr(server, "name_spans", lambda text: [(text.index(n), text.index(n) + len(n)) for n in names])
    translator = FakeTranslator(reply)
    masked, kept = server.protect(sentence)
    return server.translate_sentence(masked, translator, FakeSP(), kept, keep), translator


@pytest.mark.parametrize(
    "text,kept",
    [
        ("see https://x.co/a?b=1, ok", ["https://x.co/a?b=1"]),
        ("hey @Dana!", ["@Dana"]),
        ("at 18:30 or 1555-0142", ["18:30", "1555-0142"]),
        ("nice 🎉", ["🎉"]),
    ],
)
def test_protect_hides_what_must_not_reach_the_model(text, kept):
    masked, words = server.protect(text)
    assert list(words.values()) == kept
    assert not any(k in masked for k in kept)


@pytest.mark.parametrize(
    "text,count",
    [("Hi there. How are you?", 2), ("see x.co now", 1), ("well...no", 1), ("one\ntwo", 2), ("wow!!! ok", 2)],
)
def test_sentences(text, count):
    assert len(server.SENTENCE.findall(text)) == count


@pytest.mark.parametrize(
    "out",
    ["go to XX0 now", "go to X0 now", "go to Х0 now", "go to Χ0 now", "go to XX٠ now"],
    ids=["plain", "shortened", "cyrillic", "greek", "arabic digit"],
)
def test_restore_finds_the_token_however_the_model_wrote_it(out):
    assert server.restore(out, {"XX0": "Vancouver"}, "go to XX0 now") == "go to Vancouver now"


def test_restore_puts_a_lost_token_last():
    assert server.restore("go now", {"XX0": "Dana"}, "go XX0 now") == "go now Dana"


def test_restore_treats_a_stray_number_as_the_lost_token():
    assert server.restore("at 7 tonight", {"XX0": "18:30"}, "at XX0 tonight") == "at 18:30 tonight"


def test_restore_reads_arabic_twenty_as_the_lost_token():
    twenty = "العشرين"
    assert server.restore(f"في {twenty} مساء", {"XX0": "18:30"}, "at XX0") == (
        "في 18:30 مساء"
    )


def test_restore_keeps_a_twenty_the_sentence_really_said():
    twenty = "عشرون"
    assert server.restore(f"{twenty} قطة", {"XX0": "Dana"}, "twenty cats XX0") == (
        f"{twenty} قطة Dana"
    )


def test_restore_keeps_a_twentieth_the_sentence_really_said():
    twenty = "العشرين"
    assert server.restore(f"القرن {twenty}", {"XX0": "Dana"}, "the twentieth century XX0") == (
        f"القرن {twenty} Dana"
    )


def test_restore_replaces_a_token_the_model_wrote_twice():
    assert server.restore("XX0 et XX0", {"XX0": "Dana"}, "XX0 now") == "Dana et Dana"


def test_restore_never_searches_inside_restored_words():
    out = server.restore("meet at XX0", {"XX0": "10:30", "XX1": "https://x.co"}, "meet at XX0 XX1")
    assert out == "meet at 10:30 https://x.co"


def test_restore_ignores_x1_inside_a_restored_link():
    out = server.restore("XX0 here", {"XX0": "https://a.co/x1", "XX1": "Dana"}, "XX0 XX1 here")
    assert out == "https://a.co/x1 here Dana"


def test_translate_joins_pieces_and_drops_word_markers(monkeypatch):
    out, translator = run("Good morning", ["▁Bon", "jour", "▁mon", "de"], monkeypatch)
    assert out == "Bonjour monde"
    assert translator.options["disable_unk"] is True


def test_names_never_reach_the_model(monkeypatch):
    out, translator = run("Dana is in Paris", ["▁XX0", "▁est", "▁XX1"], monkeypatch, ["Dana", "Paris"])
    assert "Dana" not in translator.seen and "Paris" not in translator.seen
    assert set(out.split()) == {"Dana", "Paris", "est"}


def test_a_member_name_is_kept_though_the_tagger_missed_it(monkeypatch):
    out, translator = run("I saw Rose today", ["\u2581vu", "\u2581XX0"], monkeypatch, keep=["Rose"])
    assert "Rose" not in translator.seen
    assert out == "vu Rose"


def test_member_names_match_whole_words_with_their_capitals():
    assert server.member_spans("a rose, Roseanne and @Rose", ["Rose"]) == []


def test_a_name_that_starts_a_sentence_stays_with_the_tagger():
    assert server.member_spans("May I come?", ["May"]) == []
    assert server.member_spans("Ask May now", ["May"]) == [(4, 7)]


def test_joined_makes_neighbours_and_overlaps_one():
    text = "see Dana Levi now"
    assert server.joined(text, [(9, 13), (4, 8), (4, 13)]) == [(4, 13)]
    assert server.joined("a b c", [(0, 1), (4, 5)]) == [(0, 1), (4, 5)]


def test_trailing_emoji_stay_out_of_the_model(monkeypatch):
    out, translator = run("Good morning 🎉", ["▁Bonjour"], monkeypatch)
    assert out == "Bonjour 🎉"
    assert "XX" not in translator.seen


def test_punctuation_after_a_trailing_emoji_stays(monkeypatch):
    out, _ = run("Good morning 🎉!", ["▁Bonjour"], monkeypatch)
    assert out == "Bonjour 🎉!"


def test_a_token_stuck_to_a_name_comes_back_inside_it(monkeypatch):
    out, _ = run("I love my iPhone15 a lot", ["▁XX0", "▁beaucoup"], monkeypatch, ["iPhoneXX0"])
    assert out == "iPhone15 beaucoup"


def test_a_sentence_with_no_words_skips_the_model(monkeypatch):
    out, translator = run("18:30 🎉", ["unused"], monkeypatch)
    assert out == "18:30 🎉"
    assert translator.seen is None


def test_hebrew_prefix_hyphen_is_closed_up(monkeypatch):
    out, _ = run("in Paris", ["▁ב-", "▁XX0"], monkeypatch, ["Paris"])
    assert out == "ב-Paris"


def test_keep_reaches_the_translation(monkeypatch):
    monkeypatch.setattr(server, "translate", lambda path, text, keep: ",".join(keep))
    assert server.answer('{"pack": "/p", "text": "hi", "keep": ["Dana", "Rose"]}') == server.Answer(text="Dana,Rose")


def test_a_good_request_is_answered(monkeypatch):
    monkeypatch.setattr(server, "translate", lambda path, text, keep: f"{path}:{text}")
    assert server.answer('{"pack": "/p", "text": "hi"}') == server.Answer(text="/p:hi")


@pytest.mark.parametrize(
    "line",
    ['{"pack": "/p"}', '{"pack": "", "text": "x"}', '{"pack": 1, "text": "x"}', '{"pack": "/p", "text": "x", "y": 1}', "nope"],
)
def test_a_bad_request_is_an_error_not_a_crash(line):
    assert server.answer(line).error


def test_an_error_never_repeats_the_message():
    got = server.answer('{"pack": 1, "text": "my private words"}')
    assert "private" not in got.error


def test_a_failed_translation_is_an_error(monkeypatch):
    def boom(path, text, keep):
        raise RuntimeError("no model")

    monkeypatch.setattr(server, "translate", boom)
    assert server.answer('{"pack": "/p", "text": "x"}').error == "no model"


def test_an_error_with_no_text_is_still_an_error(monkeypatch):
    def boom(path, text, keep):
        raise MemoryError

    monkeypatch.setattr(server, "translate", boom)
    assert server.answer('{"pack": "/p", "text": "x"}').error == "MemoryError"


def test_the_pipe_is_utf8_whatever_the_system_uses():
    """Runs the real main loop, as on a Windows code page."""
    here = os.path.dirname(os.path.abspath(__file__))
    code = "import server; server.translate = lambda path, text, keep: text[::-1]; server.main()"
    lines = ['{"pack": "/p", "text": "שלום"}', "nope", '{"pack": "/p", "text": "Привет"}']
    done = subprocess.run(
        [sys.executable, "-c", code],
        cwd=here,
        input="\n".join(lines).encode("utf-8") + b"\n",
        capture_output=True,
        check=True,
        env={**os.environ, "PYTHONIOENCODING": "ascii", "PYTHONUTF8": "0", "PYTHONDONTWRITEBYTECODE": "1"},
        timeout=60,
    )
    answers = [json.loads(line) for line in done.stdout.decode("utf-8").splitlines()]
    assert len(answers) == 3, done.stderr
    assert answers[0] == {"text": "םולש"}
    assert "error" in answers[1]
    assert answers[2] == {"text": "тевирП"}
