#!/usr/bin/env python3
"""Build a B2/B2+ vocabulary candidate list from local Anki decks.

This is a manual helper. It does not run in the app, and it does not write to
the database. The output is a tab-separated list of:

    word<TAB>tier<TAB>cefr

That file can be passed to the content generator with -words. The tier is a
curriculum hint (1 is the most useful) and the CEFR is taken from the Oxford
5000 list when the word is there.

Requirements:
    pip install wordfreq

The Oxford 5000 CSV is public and can be downloaded once:

    curl -sSL -o /tmp/oxford-5k.csv \\
      https://raw.githubusercontent.com/nalgeon/words/main/data/oxford-5k.csv

Example:
    python3 scripts/anki_words.py --oxford /tmp/oxford-5k.csv \\
      --limit 800 --out content/words/anki-b2.txt
"""

import argparse
import csv
import json
import os
import re
import sqlite3
import tempfile
import zipfile
from collections import Counter

from wordfreq import zipf_frequency

SEED = os.path.join(os.path.dirname(__file__), "..", "backend", "internal", "store", "seed", "exercises.json")

DEFAULT_DECKS = [
    ("cambridge", os.path.expanduser("~/Downloads/Cambridge_Vocabulary_for_IELTS_Advanced.apkg")),
    ("ielts01", os.path.expanduser("~/Downloads/IELTS_vocabulary_01.apkg")),
    ("ielts", os.path.expanduser("~/Downloads/IELTS_Vocabulary.apkg")),
]

# Too informal for this product, or archaic forms that a frequency list ranks
# surprisingly high because of literature.
BLOCK = {
    "fuck", "fucking", "shit", "bitch", "cunt", "whore", "porn", "porno",
    "penis", "vagina", "rape", "raped", "rapist",
    "thou", "thee", "thy", "thine", "ye", "hath", "doth", "wherefore",
    "hither", "thither", "whence", "behold",
}

WORD_RE = re.compile(r"^[a-z][a-z'-]*$")


def clean(value: str) -> str:
    value = re.sub(r"\[sound:[^\]]+\]", "", value)
    value = re.sub(r"<[^>]+>", " ", value)
    value = value.replace("&nbsp;", " ").replace("&amp;", "&")
    return re.sub(r"\s+", " ", value).strip()


def read_deck(path: str, kind: str):
    """Return (word, definition, example) tuples from an Anki package."""
    with zipfile.ZipFile(path) as zf:
        name = next(
            n for n in zf.namelist()
            if n.startswith("collection.anki2") or n.startswith("collection.anki21")
        )
        with tempfile.TemporaryDirectory(prefix="anki_") as tmp:
            zf.extract(name, tmp)
            con = sqlite3.connect(os.path.join(tmp, name))
            try:
                rows = con.execute("select flds from notes").fetchall()
            finally:
                con.close()

    for (fields,) in rows:
        parts = [clean(p) for p in fields.split("\x1f")]
        if kind == "cambridge":
            word = parts[0] if parts else ""
            definition = parts[4] if len(parts) > 4 else ""
            example = parts[5] if len(parts) > 5 else ""
        elif kind == "ielts":
            word = parts[1] if len(parts) > 1 else ""
            definition = parts[0] if parts else ""
            example = ""
        else:
            word = parts[0] if parts else ""
            definition = parts[1] if len(parts) > 1 else ""
            example = parts[2] if len(parts) > 2 else ""
        word = word.strip().strip(".,;:").lower()
        if word:
            yield word, definition, example


def british(word: str) -> str:
    """Conservative American to British spelling conversion for headwords."""
    exact = {
        "analyze": "analyse", "analyzed": "analysed", "analyzing": "analysing",
        "paralyze": "paralyse", "paralyzed": "paralysed", "catalyze": "catalyse",
        "color": "colour", "colors": "colours", "colored": "coloured",
        "behavior": "behaviour", "behaviors": "behaviours",
        "favor": "favour", "favors": "favours", "favored": "favoured",
        "favorite": "favourite", "favorites": "favourites",
        "honor": "honour", "honors": "honours", "honored": "honoured",
        "labor": "labour", "labors": "labours",
        "neighbor": "neighbour", "neighbors": "neighbours",
        "rumor": "rumour", "rumors": "rumours",
        "humor": "humour", "vapor": "vapour", "endeavor": "endeavour",
        "flavor": "flavour", "flavors": "flavours", "harbor": "harbour",
        "center": "centre", "centers": "centres", "centered": "centred",
        "meter": "metre", "meters": "metres", "liter": "litre", "liters": "litres",
        "theater": "theatre", "theaters": "theatres", "fiber": "fibre",
        "fibers": "fibres", "caliber": "calibre", "somber": "sombre",
        "defense": "defence", "offense": "offence", "pretense": "pretence",
        "catalog": "catalogue", "dialog": "dialogue", "analog": "analogue",
        "skeptic": "sceptic", "skeptical": "sceptical", "skepticism": "scepticism",
        "maneuver": "manoeuvre", "maneuvers": "manoeuvres",
        "gray": "grey", "plow": "plough", "aging": "ageing",
        "traveler": "traveller", "travelers": "travellers",
        "traveled": "travelled", "traveling": "travelling",
        "canceled": "cancelled", "canceling": "cancelling",
        "modeled": "modelled", "modeling": "modelling",
        "labeled": "labelled", "labeling": "labelling",
        "fueled": "fuelled", "fueling": "fuelling",
    }
    if word in exact:
        return exact[word]

    # -ize/-ization is the main regular pattern. Keep words where -ize is part
    # of the root rather than a suffix (size, prize, seize and similar).
    keep = {"size", "prize", "seize", "capsize", "assize", "maize", "blitz"}
    if word in keep:
        return word
    for suffix, repl in (
        ("ization", "isation"), ("izations", "isations"),
        ("izing", "ising"), ("ized", "ised"), ("izer", "iser"), ("izers", "isers"),
        ("izes", "ises"), ("ize", "ise"),
    ):
        if word.endswith(suffix) and len(word) > len(suffix) + 3:
            return word[: -len(suffix)] + repl
    return word


def stems(word: str):
    """Cheap base-form guesses, used only to avoid near-duplicate headwords."""
    out = set()
    if word.endswith("ies") and len(word) > 4:
        out.add(word[:-3] + "y")
    if word.endswith("es") and len(word) > 4:
        out.add(word[:-2])
    if word.endswith("s") and not word.endswith("ss") and len(word) > 3:
        out.add(word[:-1])
    if word.endswith("ed") and len(word) > 4:
        out.add(word[:-2])
        out.add(word[:-1])
    if word.endswith("ing") and len(word) > 5:
        out.add(word[:-3])
        out.add(word[:-3] + "e")
    if word.endswith("ly") and len(word) > 4:
        out.add(word[:-2])
    for suffix in ("ness", "ment", "ity", "ance", "ence"):
        if word.endswith(suffix) and len(word) > len(suffix) + 3:
            out.add(word[: -len(suffix)])
    for suffix, repl in (("tion", "te"), ("sion", "t"), ("ation", "ate")):
        if word.endswith(suffix) and len(word) > len(suffix) + 3:
            out.add(word[: -len(suffix)] + repl)
    return out


def tier_for(zipf: float, cefr: str, opal: bool, source: str) -> int:
    if opal:
        if zipf >= 4.0:
            return 1
        if zipf >= 3.5:
            return 2
        return 3
    if source == "cambridge":
        return 3 if zipf >= 3.6 else 4
    if cefr == "B2" and zipf >= 4.2:
        return 2
    return 3


def read_oxford(path: str):
    levels = {}
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            word = row["word"].strip().lower()
            level = row["level"].strip().upper()
            if word and level:
                levels.setdefault(word, set()).add(level)
    out = {}
    for word, values in levels.items():
        useful = values & {"B2", "C1"}
        if useful:
            # Prefer B2 when the word has a useful B2 sense.
            out[word] = "B2" if "B2" in useful else "C1"
        else:
            out[word] = None  # present, but only A1/A2/B1
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--limit", type=int, default=800, help="maximum number of words (default 800)")
    ap.add_argument("--min-zipf", type=float, default=2.8)
    ap.add_argument("--max-zipf", type=float, default=5.0)
    ap.add_argument("--oxford", default="", help="path to the Oxford 5000 CSV; omit for a frequency-only list")
    ap.add_argument("--opal", default="", help="path to the Oxford Phrasal Academic Lexicon CSV; optional")
    ap.add_argument("--out", default="content/words/anki-b2.txt")
    ap.add_argument("--deck", action="append", default=[], metavar="NAME=PATH",
                    help="override or add a deck; NAME is cambridge, ielts01 or ielts")
    args = ap.parse_args()

    decks = dict(DEFAULT_DECKS)
    for item in args.deck:
        name, _, path = item.partition("=")
        if not path:
            ap.error("--deck must be NAME=PATH")
        decks[name] = path

    oxford = read_oxford(args.oxford) if args.oxford else {}
    opal = set()
    if args.opal:
        with open(args.opal, newline="") as f:
            for row in csv.DictReader(f):
                word = row["word"].strip().lower()
                if WORD_RE.fullmatch(word):
                    opal.add(word)

    seed = json.load(open(SEED))
    existing = {x["target_word"].lower() for x in seed if x["type"] == "vocab"}

    seen = set()
    candidates = []
    for kind in ("cambridge", "ielts01", "ielts"):
        if kind not in decks or not os.path.exists(decks[kind]):
            print(f"skip missing deck: {kind} {decks.get(kind, '')}")
            continue
        for raw, definition, example in read_deck(decks[kind], kind):
            word = british(raw)
            if word in seen or word in existing or word in BLOCK:
                continue
            if not WORD_RE.fullmatch(word) or len(word) < 3:
                continue
            zipf = zipf_frequency(word, "en")
            if zipf < args.min_zipf or zipf > args.max_zipf:
                continue

            cefr = None
            if oxford:
                cefr = oxford.get(word)
                if word in oxford and cefr is None:
                    # The word is in the Oxford list but only at A1/A2/B1.
                    continue
                if word not in oxford:
                    # Words outside the Oxford 5000 are only accepted from the
                    # Cambridge Advanced deck, which is a curated IELTS source.
                    if kind != "cambridge":
                        continue
                    cefr = "C1"
            else:
                cefr = "C1"

            seen.add(word)
            candidates.append({
                "word": word, "raw": raw, "source": kind, "zipf": round(zipf, 2),
                "cefr": cefr, "opal": word in opal,
                "definition": definition[:400], "example": example[:400],
            })

    # Academic words first, then the curated Cambridge Advanced deck, then the
    # two IELTS dictionaries. Inside each group the more frequent words come
    # first so the curriculum starts with the most useful material.
    order = {"cambridge": 0, "ielts01": 1, "ielts": 2}
    candidates.sort(key=lambda c: (not c["opal"], order[c["source"]], -c["zipf"]))

    selected = []
    used_stems = set()
    for cand in candidates:
        if len(selected) >= args.limit:
            break
        # Prefer a base form when both it and an inflection are present.
        if stems(cand["word"]) & used_stems:
            continue
        used_stems.add(cand["word"])
        cand["tier"] = tier_for(cand["zipf"], cand["cefr"], cand["opal"], cand["source"])
        selected.append(cand)

    out = os.path.abspath(args.out)
    os.makedirs(os.path.dirname(out), exist_ok=True)
    with open(out, "w") as f:
        for cand in selected:
            f.write(f'{cand["word"]}\t{cand["tier"]}\t{cand["cefr"]}\n')

    audit = out.rsplit(".", 1)[0] + ".json"
    with open(audit, "w") as f:
        json.dump(selected, f, indent=2)
        f.write("\n")

    sources = Counter(c["source"] for c in selected)
    tiers = Counter(c["tier"] for c in selected)
    cefrs = Counter(c["cefr"] for c in selected)
    print(f"wrote {len(selected)} words to {out}")
    print(f"sources: {dict(sources)}")
    print(f"tiers:   {dict(sorted(tiers.items()))}")
    print(f"cefr:    {dict(sorted(cefrs.items()))}")
    print(f"audit:   {audit}")


if __name__ == "__main__":
    main()
