# IELTS Practice

A personal daily practice tool for IELTS Academic. There are two exercise types: Vocabulary in Context and Short Writing. Every answer is evaluated by Gemini, stored in PostgreSQL, and used to build your statistics.

```text
Browser -> host nginx (TLS) -> web (static SvelteKit build)
                            -> /api -> Go API -> PostgreSQL
                                          |
                                          +-> Gemini API (server side only)
```

## Getting started

You need Docker with Compose v2 and an API key from https://aistudio.google.com.

```bash
bash scripts/init-env.sh      # asks for the key (hidden), creates .env and a random database password
docker compose up --build
```

Open `http://localhost:<WEB_PORT>` once the stack is running. The default value is in `.env.example`. The first build takes a few minutes. The API runs its migrations and loads the exercise bank on startup.

If you prefer to write `.env` yourself: `cp .env.example .env`, then fill in `GEMINI_API_KEY`, `GEMINI_MODEL`, and a database password that uses **letters and numbers only**. `.env.example` is public and contains placeholders. `.env` is ignored by Git.

Stop the stack with `docker compose down`. Your data stays in the `pgdata` volume.
**Warning:** `docker compose down -v` also deletes the database.

## What to try

1. **Practice**: choose Mixed, Vocabulary or Writing, write an answer, and press "Get feedback". `Ctrl` or `Cmd` + `Enter` submits.
2. **Rewrite**: after feedback, press "Rewrite it in my own words". Your first answer and its feedback stay visible for reference.
3. **When the AI fails**: temporarily put a wrong `GEMINI_API_KEY` in `.env`, run `docker compose up -d api`, and submit an answer. You will see "Not evaluated yet", your text stays in the box, and the answer is saved. Fix the key, run the same command again, and press "Try again"; or open the answer from **History** and press "Evaluate this answer now". No duplicate attempt is created.
4. **History**: every answer, with the full feedback and, for vocabulary, the reference meaning.
5. **Statistics**: see below.

## Statistics: what each number means

- **Quality figures (verdicts, mistakes) count first attempts only.** A rewrite happens after you have seen the feedback, so counting it would make your level look better than it is.
- **Mistakes per 10 answers** counts only issues with severity "Mistake", not "Sounds unnatural" or "Optional improvement". This keeps the numbers comparable no matter how much you wrote. "Recently" means your last 10 first attempts.
- **Recent versus overall comparison** appears after 20 answers. With fewer, the page says so and draws no conclusion.
- **Score 0 to 3** (not correct, partly, mostly, correct) is a practice measure, **not an IELTS band score**.
- **Streak**: consecutive days with at least one evaluated answer, in your own time zone. It does not break while a full day without practice has not passed yet.
- **Failed evaluations** are not counted anywhere, but are reported separately.
- **Vocabulary schedule**: each word sits in one of 5 boxes. A correct answer moves it up two boxes; a mostly correct answer moves it up one box. Both are at least 1 day away, so a word you know does not reappear in the same session. A partly correct answer moves down one box and returns within 1 day. A wrong answer restarts the word at box 1 and returns after 4 hours. The waiting times are 4 hours, 1 day, 3 days, 8 days, and 21 days. "Due now" means the word is waiting for review now. "New today" counts the new words you saw for the first time today. There is no daily limit by default.
- **Priority order**: words have tiers 1 to 4. Tier 1 is the most useful vocabulary for B2/B2+ and IELTS Academic, and is introduced first. Reviews are not affected by tier: anything due comes before any new word.

## Exposure and security

| Service | Exposure | Notes |
|---|---|---|
| Site (nginx) | loopback only | host nginx is the only public entry point |
| API | loopback only | used by the web container and by curl |
| PostgreSQL | not exposed | only the API reaches the database |

- Compose publishes the site and API on the loopback interface only. On a server, put a host nginx with TLS in front of the web service. Do not publish the container ports directly.
- There is no login. HTTPS encrypts the connection but is **not authentication**. Keep the site on a private network, or add authentication (basic auth, VPN or Tailscale) before exposing it to the internet.
- The Gemini key exists only in the API container. It is never sent to the browser and never written to logs.
- The server starts at most 10 evaluations per minute (`EVAL_RATE_PER_MINUTE`), so a browser loop cannot burn the daily quota.
- New words are unlimited by default (`NEW_VOCAB_PER_DAY=0`). Set a positive value to cap new words per day in your own time zone. Due reviews are never limited.
- Logs never contain the text of your answers.

## What is stored

In your local database: the exercises (shared content), your answers, the feedback shown, **the model's raw response** (so you can inspect bad corrections), the model name, and a hash of the prompt. The exercise text and your answer are sent to Google for evaluation. On the free tier Google may use inputs to improve its products, so do not write personal data in your answers.

Tables: `exercises` (shared content, with a priority `tier`), `attempts` (immutable history, browser-generated id used as an idempotency key), `attempt_issues` (one row per issue, for statistics), `review_state` (box and next review date per word), `schema_migrations`.

## How content works

The content bank is the material the app draws exercises from. It feeds vocabulary and writing.

There are two sources:

1. **Built-in bank**: `backend/internal/store/seed/exercises.json`. It is compiled into the API image and loaded on every API start. Loading is idempotent: an existing slug is never rewritten, so you can correct tier and source metadata without touching an exercise that already has attempts.
2. **Extra batches**: `content/generated.json` and every `.json` file in `content/batches/`. The `content` folder is mounted into the API container. The API reads these at startup, after the built-in bank, validates them, and inserts only new slugs. If a file or folder is missing, the API starts normally and logs that it skipped it.

The current built-in bank has **84 vocabulary and writing exercises (70 vocabulary, 14 writing)**. There is no daily cap by default, so you can work through as much as you want in a day.

### Adding material

There is **no automatic content generation and no scheduled import**. New material appears only when you run the manual generator, or when you add a batch file and restart the API.

To generate new material with Gemini, run the manual generator:

```bash
bash scripts/generate-content.sh vocab 10
bash scripts/generate-content.sh writing 5
```

The count is optional: vocabulary defaults to 10 and writing to 5, and the maximum is 30. The script wraps the same command you can run yourself:

```bash
docker compose run --rm --user root api generate -type vocab -count 10
docker compose run --rm --user root api generate -type writing -count 5
```

The generator validates the output, drops duplicates, writes a batch file under `content/batches/`, and inserts the accepted items into the database. Nothing runs on a schedule. Review the output before trusting it: it is AI-written and **not official IELTS material**.

To add a batch by hand:

1. Create a JSON file with the same shape as the built-in bank.
2. Save it as `content/batches/<name>.json`, or replace `content/generated.json`.
3. Run `docker compose restart api`.

See `content/README.md` for the format, examples, and the validation rules.

### How exercises are selected

- **Vocabulary**: due reviews first, in random order among words that are due. If no review is due, a new word is chosen by tier, with random tie-breaking. If you set `NEW_VOCAB_PER_DAY` to a positive number, new words stop after that cap. A correct answer moves the word up two boxes and at least 24 hours away; a mostly correct answer moves it up one box and at least 24 hours away. A partly correct answer moves down one box and returns within 24 hours. A wrong answer returns to box 1 and comes back after 4 hours.
- **Skip**: asks only for material you have never seen. It never returns a word from the review schedule. When no unseen material is left, it says so instead of repeating an old word; generate more content with the script above.
- **Writing**: prompts you have never seen first, then the ones seen longest ago. Writing is not part of the spaced repetition schedule, so a prompt can eventually return after you have seen the whole bank.
- **Mixed** prefers vocabulary and falls back to writing when no word is available. The previous exercise is avoided when there is another option.

## Backup and restore

The volume is **not** a backup. To take a copy:

```bash
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > backup.sql
```

To restore into an empty database (after `docker compose down -v` and `docker compose up -d db`):

```bash
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"' < backup.sql
```

Keep copies outside the server, for example with `scp` to your laptop.

## Free tier limits

Gemini limits are per model and change over time. In this project the Flash models had 20 requests per day and the Flash Lite models had 500. That is why the default model is `gemini-3.5-flash-lite`. If a limit is reached, the app shows a clear message and keeps your answer. Check the current limits on the AI Studio rate limits page.

Feedback from a smaller model can be inaccurate. If a correction looks strange, do not trust it blindly.

## Tests and development

**Backend tests.** Unit tests need nothing. Integration tests need a database they can use; each test creates its own schema and drops it:

```bash
# Start a disposable PostgreSQL, then point TEST_DATABASE_URL at it.
export TEST_DATABASE_URL='postgres://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=disable'
cd backend
go test ./...
```

Without `TEST_DATABASE_URL`, database-backed tests are skipped and the rest still run.

**AI feedback quality.** Ten sample answers with live calls (needs Go and `.env`):

```bash
cd backend
go run ./cmd/evalcheck -v            # all
go run ./cmd/evalcheck -only w       # writing only
```

Run it again whenever you change the prompt or the model. It also prints the time per call. Add your own answers to `backend/testdata/golden.json`.

**Frontend with hot reload.** With the stack already running:

```bash
cd frontend
npm install
npm run dev          # Vite dev server, proxies /api to the API
npm run check        # type and Svelte checks
npm run build        # production build
```

The frontend uses Tailwind CSS v4 through `@tailwindcss/vite`. Styling lives in utility classes in the Svelte files; `src/app.css` only imports the font and Tailwind, defines the font theme, and sets the focus ring.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | health, checks the database too |
| GET | `/api/exercises/next?type=vocab\|writing\|any&exclude=ID&tz=YOUR_TIME_ZONE&unseen=1` | next vocabulary or writing exercise; `unseen=1` returns only material never seen before |
| POST | `/api/attempts` | submit and evaluate (idempotent by `id`) |
| GET | `/api/attempts?limit=&offset=` | history |
| GET | `/api/attempts/{id}` | one answer with full feedback |
| GET | `/api/stats?tz=YOUR_TIME_ZONE` | statistics |

**Spaced repetition** applies to vocabulary. Only the first answer to a word changes its box. Rewrites are extra practice and do not affect the schedule or the quality statistics.

**Submission flow**: the answer is saved before the AI is called, so it is never lost. If the provider fails, the same submission is retried with the same `id`, without duplicates or double evaluation. Feedback is validated (known fields, known categories, quotes that actually appear in your answer) before it is stored or shown.

## Structure

```text
docker-compose.yml        db + api + web
.env.example              public template (placeholders)
scripts/init-env.sh       creates .env
scripts/smoke.sh          quick check (spends 1 AI call)
scripts/generate-content.sh  manual vocab and writing generation
backend/                  Go API
  cmd/api                 server
  cmd/evalcheck           live AI quality check
  internal/eval           prompts, Gemini client, validation, retry
  internal/store          PostgreSQL: migrations, seed, selection, attempts, statistics
  internal/httpapi        endpoints, rate limiter
  testdata/golden.json    sample answers
frontend/                 SvelteKit (static build) + Tailwind CSS + nginx.conf
content/                  JSON batches for manual and generated material
```

## Deployment notes

The stack has been run on a Linux VM with Docker Compose v2 and a host nginx with TLS. The host nginx should proxy to the web service over loopback; the database and the API stay private.

Update routine:

```bash
cd /path/to/ielts-practice
git pull
docker compose up -d --build
```

`git pull` does not touch `.env`. After changing only files under `content/`, run `docker compose restart api`.

## What is and is not ready

**Tested here:** the Go API against a real PostgreSQL 16, including database-backed integration tests for migrations, seeding, spaced repetition, attempts, statistics and the content generator. `go vet` and `npm run check` pass. The frontend was checked in Chromium against the real local backend and again on the deployed site: practice, history and statistics. The Docker images build and the Compose stack runs locally and on the Linux VM.

**Not tested here:** real Gemini feedback was not quality-checked with this exact build in the final check, and the automated tests use a fake local AI server. If something fails on `docker compose up --build`, send the error message.

**The built-in bank** has 84 vocabulary and writing exercises (70 vocabulary, 14 writing). All of it was written by AI and has not been reviewed by a teacher. It is original practice material, **not official IELTS material**. CEFR levels are approximate. The tier-1 vocabulary was chosen as core academic and topic vocabulary at B2/B2+, based on research on vocabulary size and text coverage (roughly 3,500 to 4,000 word families for B2, 4,000 to 5,000 for comfortable IELTS Academic reading).

**Not there yet:** accounts and guest mode, difficulty adaptation, weakness detection per concept for exercise selection, Anki import, an automatic content pipeline, and authentication for public deployment.

## Next steps (Phase B)

Suggested order: use the weaknesses already recorded to choose exercises, add more material per tier, then accounts and a proper content pipeline.

## Troubleshooting

- **The `api` stops with `GEMINI_API_KEY is not set`**: the key is still a placeholder. Fix `.env` and run `docker compose up -d api`.
- **`provider rejected request: status 404` in the logs**: `GEMINI_MODEL` does not exist. Check the model names in AI Studio.
- **Rate limit message**: wait a minute. If it says the daily quota is exhausted, change the model or wait.
- **Logs**: `docker compose logs -f api`.
- **The web port is already in use**: change `WEB_PORT` in `.env`.
