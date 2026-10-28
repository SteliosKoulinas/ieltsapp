# Extra content

Αυτός ο φάκελος είναι προσαρτημένος στο container του API ως `/content`. Το API
διαβάζει το αρχείο `generated.json` κατά την εκκίνηση, μετά το built-in seed.

Για να προσθέσεις νέο υλικό:

1. Φτιάξε ή αντέγραψε ένα JSON αρχείο με το ίδιο σχήμα που έχει το
   `backend/internal/store/seed/exercises.json`.
2. Αποθήκευσέ το εδώ ως `generated.json`.
3. Τρέξε `docker compose restart api`.

Το φόρτωμα είναι ασφαλές να επαναληφθεί: ένα slug που υπάρχει ήδη δεν
ξαναγράφεται. Αν το αρχείο δεν υπάρχει, το API ξεκινά κανονικά και το σημειώνει
στα logs.

Ελάχιστο παράδειγμα για μία λέξη:

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

Κανόνες που ελέγχονται πριν μπει το υλικό:

- μοναδικό `slug`
- `type` είναι `vocab` ή `writing`
- `cefr` είναι `A2`, `B1`, `B2` ή `C1`
- `topic` δεν είναι κενό
- για `vocab`, το `context` περιέχει ακριβώς το `target_word`
- `tier` είναι 1 έως 4 (1 = πιο σημαντικό)

Το `source` δηλώνει την προέλευση. Χρησιμοποίησε `generated` για υλικό που
έφτιαξε AI, ώστε να μη μπερδεύεται με επίσημο υλικό IELTS.
