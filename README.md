# IELTS Practice

Προσωπικό εργαλείο καθημερινής εξάσκησης για IELTS Academic. Δύο τύποι ασκήσεων (Vocabulary in Context και Short Writing). Κάθε απάντηση αξιολογείται από το Gemini, αποθηκεύεται σε PostgreSQL και τροφοδοτεί τα στατιστικά σου.

```
Browser ──> nginx (frontend, port 8080) ──> Go API ──> PostgreSQL
                  static SvelteKit build      │
                  /api/* proxied              └──> Gemini API (μόνο από τον server)
```

## Εκκίνηση

Χρειάζεσαι Docker με Compose v2 και ένα API key από https://aistudio.google.com.

```bash
bash scripts/init-env.sh      # ρωτά το key (κρυφό), φτιάχνει .env και τυχαίο κωδικό για τη βάση
docker compose up --build
```

Άνοιξε **http://localhost:8080**. Την πρώτη φορά το build παίρνει λίγα λεπτά. Το API τρέχει μόνο του τα migrations και φορτώνει τις ασκήσεις.

Αν προτιμάς να φτιάξεις το `.env` μόνος σου: `cp .env.example .env` και συμπλήρωσε `GEMINI_API_KEY`, `GEMINI_MODEL` και έναν κωδικό βάσης **μόνο από γράμματα και αριθμούς**. Το `.env.example` είναι δημόσιο και έχει μόνο placeholders. Το `.env` είναι στο `.gitignore`.

Σταμάτημα: `docker compose down`. Τα δεδομένα μένουν στο volume `pgdata`.
**Προσοχή:** `docker compose down -v` σβήνει και τη βάση.

## Τι να δοκιμάσεις

1. **Practice**: διάλεξε Mixed, Vocabulary ή Writing, γράψε απάντηση, πάτα "Get feedback". `Ctrl` ή `Cmd` + `Enter` υποβάλλει.
2. **Rewrite**: μετά το feedback πάτα "Rewrite it in my own words". Η πρώτη απάντηση και το feedback μένουν ανοιχτά για αναφορά.
3. **Όταν το AI αποτυγχάνει**: βάλε προσωρινά λάθος `GEMINI_API_KEY` στο `.env`, τρέξε `docker compose up -d api` και υπέβαλε μια απάντηση. Θα δεις "Not evaluated yet", το κείμενό σου μένει στο πλαίσιο, και η απάντηση αποθηκεύεται. Διόρθωσε το key, ξανατρέξε την ίδια εντολή και πάτα "Try again", ή άνοιξε την απάντηση από το **History** και πάτα "Evaluate this answer now". Δεν δημιουργείται διπλή απάντηση.
4. **History**: όλες οι απαντήσεις, με πλήρες feedback και (για vocab) το reference meaning.
5. **Statistics**: δες παρακάτω.

## Στατιστικά: τι σημαίνει κάθε νούμερο

- **Ποιοτικά μεγέθη (verdicts, λάθη) μετράνε μόνο τις πρώτες απαντήσεις** κάθε άσκησης. Ένα rewrite γίνεται αφού είδες το feedback, οπότε αν μετρούσε θα έδειχνε καλύτερο επίπεδο από το πραγματικό.
- **Λάθη ανά 10 απαντήσεις**: μετράνε μόνο issues με severity "Mistake" (όχι "Sounds unnatural" ή "Optional improvement"), ώστε οι αριθμοί να μη ζυγίζουν διαφορετικά ανάλογα με το πόσο έγραψες. Το "Recently" είναι οι τελευταίες 10 πρώτες απαντήσεις.
- **Σύγκριση πρόσφατα/συνολικά** εμφανίζεται από 20 απαντήσεις και πάνω. Με λιγότερες, η σελίδα το λέει ρητά και δεν βγάζει συμπέρασμα.
- **Σκορ 0 έως 3** (not correct, partly, mostly, correct): μέτρο εξάσκησης και **όχι IELTS band score**.
- **Streak**: συνεχόμενες μέρες με τουλάχιστον μία αξιολογημένη απάντηση, στη δική σου ζώνη ώρας. Δεν σπάει όσο δεν έχει περάσει ολόκληρη μέρα χωρίς εξάσκηση.
- **Αποτυχημένες αξιολογήσεις** δεν μετράνε πουθενά, αλλά αναφέρονται ξεχωριστά.
- **Vocabulary schedule**: κάθε λέξη μπαίνει σε ένα από 5 boxes. Σωστή ή σχεδόν σωστή απάντηση (`correct`, `mostly_correct`) ανεβάζει box και η λέξη δεν ξαναεμφανίζεται για τουλάχιστον 1 μέρα: 1 μέρα, 3 μέρες, 8 μέρες, 21 μέρες. Έτσι μια λέξη που την ξέρεις δεν ξαναβγαίνει στο ίδιο session. Λάθος ή μερική απάντηση (`incorrect`, `partially_correct`) γυρίζει τη λέξη πίσω, ώστε να ξαναέρθει σύντομα: στο box 1 σε 10 λεπτά. "Due now" σημαίνει ότι η λέξη περιμένει επανάληψη τώρα. "New today" μετράει πόσες νέες λέξεις πρωτοείδες σήμερα, με ανώτατο όριο `NEW_VOCAB_PER_DAY`.
- **Σειρά προτεραιότητας**: οι λέξεις έχουν tier 1 έως 4. Το tier 1 είναι το πιο χρήσιμο λεξιλόγιο για B2/B2+ και IELTS Academic, και μπαίνει πρώτο. Οι επαναλήψεις δεν επηρεάζονται από το tier: ό,τι είναι due έρχεται πριν από κάθε νέα λέξη.

## Πύλες και ασφάλεια

| Υπηρεσία | Διεύθυνση | Σχόλιο |
|---|---|---|
| Site (nginx) | http://localhost:8080 | μόνο 127.0.0.1 |
| API | http://localhost:8081 | μόνο 127.0.0.1, για curl και για `npm run dev` |
| PostgreSQL | καμία | μόνο το API φτάνει στη βάση |

- Τίποτα δεν είναι προσβάσιμο από άλλα μηχανήματα. Δεν υπάρχει login, οπότε **μην το ανοίξεις δημόσια** χωρίς πρώτα authentication (βλέπε "Επόμενα βήματα").
- Το Gemini key υπάρχει μόνο στο container του API, δεν μπαίνει στο frontend και δεν γράφεται σε logs.
- Ο server ξεκινά το πολύ 10 αξιολογήσεις το λεπτό (`EVAL_RATE_PER_MINUTE`), για να μην καίει ο browser την ημερήσια ποσόστωση σε περίπτωση βρόχου.
- Νέες λέξεις μπαίνουν το πολύ 8 την ημέρα (`NEW_VOCAB_PER_DAY`), στη δική σου ζώνη ώρας. Οι επαναλήψεις που είναι due δεν περιορίζονται.
- Τα logs δεν περιέχουν ποτέ το κείμενο των απαντήσεών σου.

## Τι αποθηκεύεται

Στην τοπική βάση σου: οι ασκήσεις (κοινό περιεχόμενο), οι απαντήσεις σου, το feedback που εμφανίστηκε, **και η ακατέργαστη απάντηση του μοντέλου** (για να ελέγχεις κακές διορθώσεις), το όνομα του μοντέλου και ένα hash του prompt. Το κείμενο της άσκησης και της απάντησής σου στέλνεται στο Google για αξιολόγηση. Στο free tier η Google μπορεί να χρησιμοποιεί τις εισόδους για βελτίωση των προϊόντων της, οπότε μη γράφεις προσωπικά δεδομένα στις απαντήσεις.

Πίνακες: `exercises` (κοινό περιεχόμενο, με `tier` προτεραιότητας), `attempts` (αμετάβλητο ιστορικό, id από τον browser ως idempotency key), `attempt_issues` (μία γραμμή ανά issue, για τα στατιστικά), `review_state` (box και επόμενη ημερομηνία επανάληψης ανά λέξη), `schema_migrations`.

### Προσθήκη νέου υλικού

Το built-in υλικό είναι στο `backend/internal/store/seed/exercises.json`. Για νέα παρτίδα χωρίς rebuild, βάλε ένα JSON με το ίδιο σχήμα στο `content/generated.json` και τρέξε `docker compose restart api`. Το API το διαβάζει μετά το built-in seed, ελέγχει τα πεδία και δεν ξαναγράφει slugs που υπάρχουν ήδη. Αν το αρχείο λείπει, το API ξεκινά κανονικά. Οδηγίες και παράδειγμα: `content/README.md`.

### Backup και επαναφορά
Το volume **δεν είναι backup**. Αντίγραφο ασφαλείας:

```bash
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > backup.sql
```

Επαναφορά σε άδεια βάση (μετά από `docker compose down -v` και `docker compose up -d db`):

```bash
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"' < backup.sql
```

## Όρια του free tier

Τα όρια του Gemini είναι ανά μοντέλο και αλλάζουν. Στο project σου, τα Flash μοντέλα είχαν 20 κλήσεις την ημέρα και τα Flash Lite 500. Γι' αυτό το προεπιλεγμένο μοντέλο είναι `gemini-3.5-flash-lite`. Αν ένα όριο χτυπήσει, η εφαρμογή δείχνει καθαρό μήνυμα και κρατά την απάντησή σου. Δες τα τρέχοντα όρια στη σελίδα rate limits του AI Studio.

Το feedback ενός μικρότερου μοντέλου μπορεί να έχει ανακρίβειες. Αν μια διόρθωση σου φαίνεται περίεργη, μην την εμπιστευτείς τυφλά.

## Δοκιμές και ανάπτυξη

**Backend tests.** Οι unit tests δεν χρειάζονται τίποτα. Τα integration tests χρειάζονται μια βάση που μπορούν να χρησιμοποιήσουν (κάθε test φτιάχνει δικό του schema και το σβήνει):

```bash
docker run --rm -d --name pgtest -e POSTGRES_PASSWORD=test -p 55432:5432 postgres:16-alpine
cd backend
TEST_DATABASE_URL='postgres://postgres:test@127.0.0.1:55432/postgres?sslmode=disable' go test ./...
docker rm -f pgtest
```

Χωρίς `TEST_DATABASE_URL` τα DB tests παραλείπονται (skip) και τα υπόλοιπα τρέχουν.

**Ποιότητα του AI feedback.** Δέκα δοκιμαστικές απαντήσεις με ζωντανές κλήσεις (χρειάζεται Go και το `.env`):

```bash
cd backend
go run ./cmd/evalcheck -v            # όλα
go run ./cmd/evalcheck -only w       # μόνο τα writing
```

Τρέξ' το ξανά κάθε φορά που αλλάζεις prompt ή μοντέλο. Τυπώνει και χρόνους ανά κλήση. Πρόσθεσε δικές σου απαντήσεις στο `backend/testdata/golden.json`.

**Frontend με hot reload.** Με το stack ήδη σηκωμένο:

```bash
cd frontend
npm install
npm run dev          # http://localhost:5173, το /api πάει στο 127.0.0.1:8081
npm run check        # έλεγχος τύπων
```

## API

| Μέθοδος | Διαδρομή | Σκοπός |
|---|---|---|
| GET | `/api/health` | υγεία, ελέγχει και τη βάση |
| GET | `/api/exercises/next?type=vocab\|writing\|any&exclude=ID&tz=Europe/Athens` | επόμενη άσκηση |
| POST | `/api/attempts` | υποβολή και αξιολόγηση (idempotent με το `id`) |
| GET | `/api/attempts?limit=&offset=` | ιστορικό |
| GET | `/api/attempts/{id}` | μία απάντηση με πλήρες feedback |
| GET | `/api/stats?tz=Europe/Athens` | στατιστικά |

**Επιλογή επόμενης άσκησης**: για vocabulary, πρώτα η παλαιότερη λέξη που είναι due, μετά μια νέα λέξη αν δεν έχει καλυφθεί το ημερήσιο όριο, με σειρά tier και τυχαία επιλογή σε ισοπαλία. Για writing, πρώτα όσες δεν έχεις δει ποτέ, μετά όποια έχεις δει πιο παλιά. Το "Mixed" προτιμά vocabulary και πέφτει σε writing όταν δεν υπάρχει διαθέσιμη λέξη. Η προηγούμενη άσκηση αποφεύγεται όταν υπάρχει άλλη επιλογή.

**Spaced repetition**: μόνο οι λέξεις μπαίνουν σε πρόγραμμα επανάληψης. Μόνο η πρώτη απάντηση σε κάθε λέξη αλλάζει το box. Τα rewrites είναι πρόσθετη εξάσκηση και δεν επηρεάζουν το πρόγραμμα ούτε τα ποιοτικά στατιστικά.

**Ροή υποβολής**: η απάντηση αποθηκεύεται πριν κληθεί το AI, ώστε να μη χάνεται ποτέ. Αν αποτύχει ο provider, η ίδια υποβολή ξαναδοκιμάζεται με το ίδιο `id` χωρίς διπλότυπα ή διπλή αξιολόγηση. Το feedback περνά επικύρωση (σωστά πεδία, γνωστές κατηγορίες, παραθέματα που υπάρχουν πράγματι στην απάντησή σου) πριν αποθηκευτεί ή εμφανιστεί.

## Δομή

```
docker-compose.yml        db + api + web
.env.example              δημόσιο template (placeholders)
scripts/init-env.sh       δημιουργεί το .env
scripts/smoke.sh          γρήγορος έλεγχος (ξοδεύει 1 κλήση AI)
backend/                  Go API
  cmd/api                 server
  cmd/evalcheck           έλεγχος ποιότητας AI με ζωντανές κλήσεις
  internal/eval           prompts, Gemini client, επικύρωση, retry
  internal/store          PostgreSQL: migrations, seed, επιλογή, attempts, στατιστικά
  internal/httpapi        endpoints, rate limiter
  testdata/golden.json    δοκιμαστικές απαντήσεις
frontend/                 SvelteKit (static build) + nginx.conf
content/                  extra JSON batches, mounted read-only into the api
```

## Τι είναι και τι δεν είναι έτοιμο

**Ελέγχθηκε εδώ:** το Go API με πραγματικό PostgreSQL 16 (42 Go tests, με database-backed integration tests για migrations, seed, spaced repetition, attempts και στατιστικά), ολόκληρη η στοίβα σε πραγματικό headless Chromium με 27 ελέγχους (πρακτική, αποτυχία και retry, ιστορικό, στατιστικά, κινητό, γραμματοσειρά), το `nginx.conf` με πραγματικό nginx 1.24, το `npm ci` και το build του frontend. Το AI στις δοκιμές αυτές ήταν **ψεύτικος τοπικός server**, όχι το πραγματικό Gemini. Οι νέες οθόνες του spaced repetition δεν έχουν ελεγχθεί ακόμα σε browser.

**Δεν ελέγχθηκε εδώ:** η εκτέλεση των `docker build` και `docker compose` (δεν υπήρχε Docker). Τα Dockerfiles και το compose ακολουθούν τα βήματα που δοκιμάστηκαν χωριστά, αλλά αν κάτι αποτύχει στο πρώτο `docker compose up --build`, στείλε μου το μήνυμα. Επίσης δεν δοκιμάστηκε το πραγματικό Gemini με αυτό τον κώδικα, οπότε το πρώτο feedback θα δείξει αν οι αξιολογήσεις είναι καλές.

**Οι 84 ασκήσεις** (70 vocabulary, 14 writing) γράφτηκαν από AI και δεν έχουν ελεγχθεί από δάσκαλο. Είναι πρωτότυπες ασκήσεις εξάσκησης, **όχι επίσημο υλικό IELTS**. Το επίπεδο CEFR είναι προσεγγιστικό. Οι 50 νέες λέξεις tier 1 επιλέχθηκαν ως βασικό ακαδημαϊκό και θεματικό λεξιλόγιο B2/B2+, με βάση την έρευνα για μέγεθος λεξιλογίου και κάλυψη κειμένου (περίπου 3.500 έως 4.000 word families για B2, 4.000 έως 5.000 για άνετη κατανόηση IELTS Academic reading).

**Δεν υπάρχουν ακόμα:** λογαριασμοί και guest mode, προσαρμογή δυσκολίας, ανίχνευση αδυναμιών ανά concept για επιλογή ασκήσεων, εισαγωγή Anki, αυτόματο pipeline περιεχομένου, TLS και CSP για δημόσιο deploy.

## Επόμενα βήματα (Φάση Β)

Η προτεινόμενη σειρά: χρήση των αδυναμιών που ήδη καταγράφονται για την επιλογή ασκήσεων, περισσότερο υλικό ανά tier, και μετά λογαριασμοί και deploy σε Linux VM με nginx. Το `frontend/nginx.conf` δουλεύει και εκεί: άλλαξε το `api:8080` στη διεύθυνση του Go service και πρόσθεσε TLS.

## Προβλήματα

- **Το `api` σταματά με `GEMINI_API_KEY is not set`**: το key είναι ακόμα placeholder. Διόρθωσε το `.env` και τρέξε `docker compose up -d api`.
- **`provider rejected request: status 404` στα logs**: το `GEMINI_MODEL` δεν υπάρχει. Δες τα ονόματα στο AI Studio.
- **Μήνυμα για rate limit**: περίμενε ένα λεπτό. Αν λέει ότι εξαντλήθηκε η ημερήσια ποσόστωση, άλλαξε μοντέλο ή περίμενε.
- **Logs**: `docker compose logs -f api`.
- **Η πόρτα 8080 είναι πιασμένη**: άλλαξε το `WEB_PORT` στο `.env`.
