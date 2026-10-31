# Extra content

This folder is mounted into the API container as `/content`. It has two jobs:

1. **Startup batches.** The API reads `generated.json` and every `.json` file in
   `batches/` at startup, after the built-in seed. This is how you add material
   by hand.
2. **Manual generation output.** The `api generate` command writes each new
   batch into `batches/` and inserts it into the database straight away.

Loading is safe to repeat: a slug that already exists is not rewritten. If a
file or folder is missing, the API starts normally and notes it in the logs.
Nothing is generated on a schedule.

## Generate new material

The generator calls Gemini once per command, validates the output, drops
duplicates, writes a batch file, and inserts the accepted items. Run it from the
project folder:

```bash
docker compose run --rm --user root api generate -type vocab -count 10
docker compose run --rm --user root api generate -type writing -count 5
```

- `-count` is how many **new** valid items to keep, from 1 to 30. The model is
  asked for a few extras so that duplicates and invalid items can be dropped.
- `-out DIR` changes the batch folder. The default is `/content/batches`.
- `--user root` is needed because the container runs as an unprivileged user and
  the mounted folder belongs to the host user.

The batch file stays on disk even if insertion fails, so a failed run does not
waste the generation. Load it later by restarting the API, or rerun the
generator (it will skip the slugs that are already present).

Review new material before trusting it. It is AI-written and is **not official
IELTS material**.

## Add material by hand

1. Create a JSON file with the shape shown below.
2. Save it as `content/batches/<name>.json`, or replace `content/generated.json`.
3. Run `docker compose restart api`.

Batches are read in file name order. A bad file stops startup with a message
naming the file and the item, so fix it before starting again.

## JSON shapes

Every item needs `slug`, `type`, `topic`, `cefr`, `tier` and `source`. `tier` is
1 to 4, where 1 is the most useful material for a learner targeting IELTS 6.5.
`source` records provenance: use `generated` for AI-written material.

### Vocabulary

```json
[
  {
    "slug": "vocab-resilient",
    "type": "vocab",
    "context": "Cities need resilient systems that can cope with extreme weather.",
    "target_word": "resilient",
    "intended_meaning": "able to recover quickly from difficulties or change",
    "topic": "environment",
    "cefr": "B2",
    "tier": 1,
    "source": "generated"
  }
]
```

### Writing

```json
[
  {
    "slug": "writing-public-transport",
    "type": "writing",
    "prompt": "Some cities make public transport free. Do you think this is a good idea? Give one reason.",
    "expected_length": "2 to 3 sentences",
    "criteria": ["give a clear opinion", "support it with one reason"],
    "topic": "transport",
    "cefr": "B2",
    "tier": 1,
    "source": "generated"
  }
]
```

## Rules checked before insertion

- unique, lowercase `slug` using letters, digits and hyphens
- `type` is `vocab` or `writing`
- `cefr` is `A2`, `B1`, `B2` or `C1`
- `topic` is not empty
- no em dash (U+2014) in any text field
- no claim that the material is "official IELTS" content
- for `vocab`, `context` contains the exact `target_word`
- for `writing`, `prompt` is not empty
