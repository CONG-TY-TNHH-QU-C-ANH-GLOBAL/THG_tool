#!/usr/bin/env python3
"""PR8A ROOT_CAUSE_REPORT helper — read-only.

Runs the two queries from specs/ROOT_CAUSE_REPORT.md against whichever SQLite
file actually holds the comment execution_attempts, so you don't have to guess
between data/scraper.db (the server default DB_PATH) and data/local.db.

Usage (from repo root):
    python scripts/rootcause_query.py
    DB_PATH=path/to/your.db python scripts/rootcause_query.py   # explicit DB

The DB is selected from the DB_PATH environment variable (if set) and the
two known in-repo candidates only. Accepting a database path as a CLI
argument was removed deliberately: it let an arbitrary, caller-supplied
path flow straight into sqlite3.connect (sonar pythonsecurity:S8706).

It prints:
  1) phase distribution across the last 20 failed comment attempts
  2) the full Evidence Pack of the most recent failed comment attempt
Nothing is written. Safe to run as many times as you like.
"""
import json
import os
import sqlite3

FAIL_FILTER = "outcome NOT IN ('dom_verified','optimistic_success','duplicate_blocked')"


def candidate_dbs():
    # server default first (config.go DB_PATH default), then the common alt.
    env = os.environ.get("DB_PATH")
    out = []
    if env:
        out.append(env)
    out += [os.path.join("data", "scraper.db"), os.path.join("data", "local.db")]
    return out


def has_comment_attempts(path):
    if not os.path.exists(path):
        return -1
    try:
        con = sqlite3.connect(path)
        cur = con.cursor()
        cur.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='execution_attempts'")
        if not cur.fetchone():
            return -1
        n = cur.execute(
            f"SELECT COUNT(*) FROM execution_attempts WHERE action_type='comment' AND {FAIL_FILTER}"
        ).fetchone()[0]
        return n
    except sqlite3.Error:
        return -1
    finally:
        try:
            con.close()
        except Exception:
            pass


def pick_db():
    best, best_n = None, -1
    for p in candidate_dbs():
        n = has_comment_attempts(p)
        if n > best_n:
            best, best_n = p, n
    return best, best_n


def jx(evidence, path):
    """Read a dotted path out of the evidence_json blob (best-effort)."""
    try:
        obj = json.loads(evidence or "{}")
    except Exception:
        return None
    cur = obj
    for key in path.split("."):
        if not isinstance(cur, dict) or key not in cur:
            return None
        cur = cur[key]
    return cur


def main():
    db, n = pick_db()
    if db is None or n <= 0:
        print("No failed comment attempts found in any candidate DB.")
        print("Checked:", ", ".join(candidate_dbs()))
        print("Run >=1 comment_all_leads against a running server first, then re-run.")
        return
    print(f"DB = {db}   (failed comment attempts: {n})")
    con = sqlite3.connect(db)
    con.row_factory = sqlite3.Row
    cur = con.cursor()

    # ---- 1) phase distribution across last 20 failed attempts ----
    rows = cur.execute(
        f"SELECT id, evidence_json FROM execution_attempts "
        f"WHERE action_type='comment' AND {FAIL_FILTER} ORDER BY id DESC LIMIT 20"
    ).fetchall()
    dist = {}
    for r in rows:
        ph = jx(r["evidence_json"], "nav_diagnostic.phase") or "(none)"
        dist[ph] = dist.get(ph, 0) + 1
    total = len(rows)
    print(f"\n=== PHASE DISTRIBUTION (last {total} failed) ===")
    for ph, c in sorted(dist.items(), key=lambda kv: -kv[1]):
        pct = round(100 * c / total) if total else 0
        print(f"  {ph:<12} {c:>3}  ({pct}%)")
    if dist:
        dom = max(dist.items(), key=lambda kv: kv[1])[0]
        print(f"  -> dominant phase = {dom}")

    # ---- 2) full evidence pack of the most recent failed attempt ----
    print("\n=== LATEST FAILED ATTEMPT — FULL EVIDENCE PACK ===")
    r = cur.execute(
        f"SELECT id, target_url, outcome, failure_reason, evidence_json "
        f"FROM execution_attempts WHERE action_type='comment' AND {FAIL_FILTER} "
        f"ORDER BY id DESC LIMIT 1"
    ).fetchone()
    fields = [
        "nav_diagnostic.phase", "nav_diagnostic.redirect_class",
        "nav_diagnostic.landed_url", "nav_diagnostic.final_url",
        "nav_diagnostic.doc_title",
        "nav_diagnostic.article_count", "nav_diagnostic.comment_button_count",
        "nav_diagnostic.composer_count", "nav_diagnostic.textarea_count",
        "nav_diagnostic.contenteditable_count",
        "nav_diagnostic.screenshot_path",
    ]
    print(f"  id={r['id']}  outcome={r['outcome']}  failure_reason={r['failure_reason']}")
    print(f"  target_url={r['target_url']}")
    for f in fields:
        print(f"  {f.split('.')[-1]:<22} = {jx(r['evidence_json'], f)}")
    events = jx(r["evidence_json"], "nav_diagnostic.nav_events") or []
    print(f"  nav_events ({len(events)}):")
    for e in events:
        print(f"    t+{e.get('t_ms',0):>5}ms  kind={e.get('kind',''):<9} "
              f"transition={e.get('transition',''):<13} "
              f"qualifiers={e.get('qualifiers','')}  url={e.get('url','')}")
    con.close()


if __name__ == "__main__":
    main()
