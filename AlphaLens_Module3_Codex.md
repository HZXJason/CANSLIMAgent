# AlphaLens — Module Three: Trade Journal
## Codex Implementation Prompt

---

## Overview

Build the Trade Journal module for AlphaLens. This is a standalone decision-archive tool for experienced individual investors following CANSLIM methodology. It captures trade reasoning at the moment of execution and supports post-trade reflection in a separate, unlocked phase.

The module has no dependencies on Module One (Screener) or Module Two (Stock Analysis). All data is manually entered by the user.

---

## Tech Stack

- **Backend**: Go
- **Frontend**: React + TypeScript
- **Database**: PostgreSQL (consistent with existing modules)
- **API Style**: RESTful JSON

---

## Database Schema

```sql
CREATE TABLE trade_journal_entries (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  -- Phase 1: Decision fields (locked after first save)
  ticker          TEXT NOT NULL,
  direction       TEXT NOT NULL CHECK (direction IN ('buy', 'sell')),
  trade_date      DATE NOT NULL,
  price           NUMERIC(12, 4) NOT NULL,
  position_size   NUMERIC(6, 2) NOT NULL,   -- percentage, e.g. 5.00 = 5%
  stop_loss       NUMERIC(12, 4) NOT NULL,
  notes           TEXT,                      -- optional free text

  -- Phase 2: Reflection fields (always editable, all optional)
  exit_price      NUMERIC(12, 4),
  pnl_pct         NUMERIC(8, 4),             -- percentage, e.g. 12.50 = 12.5%
  reflection      TEXT,

  -- Lock flag
  phase1_locked   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_trade_journal_created_at ON trade_journal_entries (created_at DESC);
```

**Locking rule**: `phase1_locked` is set to `TRUE` on the first successful Phase 1 save. Once locked, the backend must reject any updates to Phase 1 fields.

---

## API Specification

### Base path: `/api/v1/journal`

---

### `GET /api/v1/journal`

Returns all journal entries in reverse chronological order.

**Response 200:**
```json
[
  {
    "id": "uuid",
    "ticker": "NVDA",
    "direction": "buy",
    "trade_date": "2026-06-20",
    "price": 132.50,
    "position_size": 8.00,
    "stop_loss": 119.25,
    "notes": "Breaking out of 8-week base on volume 2x average.",
    "exit_price": null,
    "pnl_pct": null,
    "reflection": null,
    "phase1_locked": true,
    "created_at": "2026-06-20T14:32:00Z",
    "updated_at": "2026-06-20T14:32:00Z"
  }
]
```

---

### `POST /api/v1/journal`

Creates a new entry with Phase 1 fields. Sets `phase1_locked = true` on success.

**Request body:**
```json
{
  "ticker": "NVDA",
  "direction": "buy",
  "trade_date": "2026-06-20",
  "price": 132.50,
  "position_size": 8.00,
  "stop_loss": 119.25,
  "notes": "Optional free text"
}
```

**Validation rules:**
- `ticker`: required, non-empty string (no symbol validation — accept any value)
- `direction`: required, must be `"buy"` or `"sell"`
- `trade_date`: required, valid date
- `price`: required, positive number
- `position_size`: required, positive number
- `stop_loss`: required, positive number
- `notes`: optional

**Response 201:** Full entry object (same shape as GET list item)

**Response 400:** Validation error
```json
{ "error": "missing required field: price" }
```

---

### `GET /api/v1/journal/:id`

Returns a single entry by ID.

**Response 200:** Full entry object
**Response 404:** `{ "error": "entry not found" }`

---

### `PATCH /api/v1/journal/:id`

Updates Phase 2 reflection fields only. Phase 1 fields must be ignored or rejected if included in the request body.

**Request body (all optional):**
```json
{
  "exit_price": 148.75,
  "pnl_pct": 12.26,
  "reflection": "Held well through first pullback. Sold into strength near pivot +10%."
}
```

**Backend rule**: If request body contains any Phase 1 field (`ticker`, `direction`, `trade_date`, `price`, `position_size`, `stop_loss`, `notes`), return 400.

**Response 200:** Updated full entry object
**Response 400:** `{ "error": "phase 1 fields are locked and cannot be modified" }`
**Response 404:** `{ "error": "entry not found" }`

---

## Frontend Specification

### Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/journal` | `JournalList` | Main list view |
| `/journal/new` | `JournalEntry` | New entry form (Phase 1) |
| `/journal/:id` | `JournalEntry` | Detail + Phase 2 reflection |

---

### `JournalList` Component

- Fetches `GET /api/v1/journal` on mount
- Renders entries as a vertical list, newest first
- Each row displays: **Ticker**, **Direction badge** (Buy = green, Sell = red), **Trade Date**, **Phase 2 indicator** (small dot or label if reflection has been added)
- "New Entry" button in top-right navigates to `/journal/new`
- Clicking a row navigates to `/journal/:id`
- Empty state: "No trades logged yet. Click New Entry to begin."

---

### `JournalEntry` Component — New Entry Mode (`/journal/new`)

Renders a form with:

**Phase 1 fields (all required except Notes):**

| Field | Input Type | Notes |
|-------|-----------|-------|
| Ticker | Text input | Uppercase on display, no validation |
| Direction | Toggle / button group | "Buy" / "Sell" |
| Trade Date | Date picker | Defaults to today |
| Price | Number input | Positive decimal |
| Position Size (%) | Number input | Positive decimal |
| Stop Loss | Number input | Positive decimal |
| Notes | Textarea | Optional |

- "Save Entry" button: disabled until all required fields are filled
- On save: `POST /api/v1/journal` → on success, navigate to `/journal/:id` for the new entry
- On validation error: display inline field-level error messages

---

### `JournalEntry` Component — Detail Mode (`/journal/:id`)

**Phase 1 section (read-only):**
- Display all Phase 1 fields as labeled key-value pairs
- No edit controls; fields are visually presented as locked (e.g., muted background)
- Label the section: "Decision" with the trade date

**Phase 2 section (editable):**
- Rendered below Phase 1 with a visual separator
- Label the section: "Reflection"
- Fields: Exit Price (optional), P&L % (optional), Reflection text (optional)
- "Save Reflection" button: always enabled (all fields optional)
- On save: `PATCH /api/v1/journal/:id`
- If Phase 2 has no data yet, show empty inputs with placeholder text

---

## Frontend State Design

```typescript
interface JournalEntry {
  id: string;
  ticker: string;
  direction: 'buy' | 'sell';
  trade_date: string;       // ISO date string "YYYY-MM-DD"
  price: number;
  position_size: number;
  stop_loss: number;
  notes: string | null;
  exit_price: number | null;
  pnl_pct: number | null;
  reflection: string | null;
  phase1_locked: boolean;
  created_at: string;
  updated_at: string;
}

interface NewEntryForm {
  ticker: string;
  direction: 'buy' | 'sell' | '';
  trade_date: string;
  price: string;            // string for controlled input, parse on submit
  position_size: string;
  stop_loss: string;
  notes: string;
}

interface ReflectionForm {
  exit_price: string;
  pnl_pct: string;
  reflection: string;
}
```

---

## Implementation Phases

### Phase 1 — Core CRUD (ship first)
1. DB schema and migration
2. Go backend: all 4 API endpoints with validation and lock enforcement
3. `JournalList` component with fetch + list rendering
4. `JournalEntry` new-entry form with Phase 1 fields
5. `JournalEntry` detail view with read-only Phase 1 display

### Phase 2 — Reflection
6. Phase 2 editable section in `JournalEntry` detail view
7. `PATCH` endpoint integration
8. Phase 2 indicator on list rows

---

## Key Constraints & Rules

- **No ticker validation**: Accept any string as a ticker symbol. Do not make external API calls to validate.
- **Phase 1 is permanently locked**: Once `phase1_locked = TRUE`, the backend must never update Phase 1 fields regardless of what is sent in a request.
- **Phase 2 fields are always optional**: Never block a PATCH save because fields are empty.
- **Phase 2 is always editable**: Unlike Phase 1, reflection fields have no locking mechanism and can be updated at any time.
- **No delete endpoint**: Omit delete functionality entirely.
- **No inter-module linking**: No entry points from Module One or Module Two. The journal is accessed only via its own route.
- **Chronological ordering**: List always sorted by `created_at DESC`. No user-controlled sort or filter.

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Required Phase 1 field missing | 400 from backend; show inline field error on frontend |
| Phase 1 field sent in PATCH | 400 with message "phase 1 fields are locked and cannot be modified" |
| Entry not found | 404; frontend shows "Entry not found" with back link |
| Network error on list load | Show error state with retry button |
| Network error on save | Show toast/inline error; do not navigate away |
