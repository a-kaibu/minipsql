package regress_test

import "testing"

func TestUpsertDoNothing(t *testing.T) {
	run(t, testGroup{
		Name: "UPSERT_DO_NOTHING",
		Setup: []string{
			"CREATE TABLE kv (key TEXT PRIMARY KEY, value INTEGER)",
			"INSERT INTO kv VALUES ('a', 1), ('b', 2), ('c', 3)",
			"INSERT INTO kv VALUES ('d', 4) ON CONFLICT DO NOTHING",
			"INSERT INTO kv VALUES ('a', 99) ON CONFLICT DO NOTHING",
		},
		Cases: map[string]testCase{
			"conflict_did_not_update": {
				SQL:  "SELECT value FROM kv WHERE key = 'a'",
				Want: [][]string{{"1"}},
			},
			"no_conflict_inserted": {
				SQL:  "SELECT value FROM kv WHERE key = 'd'",
				Want: [][]string{{"4"}},
			},
			"total_rows": {
				SQL:  "SELECT COUNT(*) FROM kv",
				Want: [][]string{{"4"}},
			},
		},
	})
}

func TestUpsertDoUpdate(t *testing.T) {
	run(t, testGroup{
		Name: "UPSERT_DO_UPDATE",
		Setup: []string{
			"CREATE TABLE counters (name TEXT PRIMARY KEY, count INTEGER)",
			"INSERT INTO counters VALUES ('views', 10), ('clicks', 5)",
			"INSERT INTO counters VALUES ('views', 1) ON CONFLICT(name) DO UPDATE SET count = counters.count + EXCLUDED.count",
			"INSERT INTO counters VALUES ('shares', 1) ON CONFLICT(name) DO UPDATE SET count = counters.count + EXCLUDED.count",
		},
		Cases: map[string]testCase{
			"verify_updated": {
				SQL:  "SELECT count FROM counters WHERE name = 'views'",
				Want: [][]string{{"11"}},
			},
			"verify_new_inserted": {
				SQL:  "SELECT count FROM counters WHERE name = 'shares'",
				Want: [][]string{{"1"}},
			},
			"verify_untouched": {
				SQL:  "SELECT count FROM counters WHERE name = 'clicks'",
				Want: [][]string{{"5"}},
			},
		},
	})
}

func TestUpsertMultipleRows(t *testing.T) {
	run(t, testGroup{
		Name: "UPSERT_MULTIPLE",
		Setup: []string{
			"CREATE TABLE inventory (sku TEXT PRIMARY KEY, qty INTEGER)",
			"INSERT INTO inventory VALUES ('A001', 10), ('A002', 20)",
			"INSERT INTO inventory VALUES ('A001', 5), ('A002', 3), ('A003', 15) ON CONFLICT(sku) DO UPDATE SET qty = inventory.qty + EXCLUDED.qty",
		},
		Cases: map[string]testCase{
			"verify_all": {
				SQL:  "SELECT sku, qty FROM inventory ORDER BY sku",
				Want: [][]string{{"A001", "15"}, {"A002", "23"}, {"A003", "15"}},
			},
		},
	})
}
