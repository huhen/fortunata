// Пакет store: хранение розыгрышей в SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	sqlite "modernc.org/sqlite" // регистрация драйвера "sqlite" для database/sql
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrDuplicate = errors.New("розыгрыш с таким номером уже существует")
	ErrNotFound  = errors.New("розыгрыш не найден")
)

// Draw — один прошедший розыгрыш.
type Draw struct {
	DrawNo    int64     `json:"drawNo"`
	Numbers   []int     `json:"numbers"` // ровно 7 чисел 1–35, хранятся по возрастанию
	Bonus     int       `json:"bonus"`   // 1–54
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Store struct{ db *sql.DB }

const schema = `CREATE TABLE IF NOT EXISTS draws (
	draw_no    INTEGER PRIMARY KEY CHECK (draw_no >= 1),
	n1 INTEGER NOT NULL CHECK (n1 BETWEEN 1 AND 35),
	n2 INTEGER NOT NULL CHECK (n2 BETWEEN 1 AND 35),
	n3 INTEGER NOT NULL CHECK (n3 BETWEEN 1 AND 35),
	n4 INTEGER NOT NULL CHECK (n4 BETWEEN 1 AND 35),
	n5 INTEGER NOT NULL CHECK (n5 BETWEEN 1 AND 35),
	n6 INTEGER NOT NULL CHECK (n6 BETWEEN 1 AND 35),
	n7 INTEGER NOT NULL CHECK (n7 BETWEEN 1 AND 35),
	bonus      INTEGER NOT NULL CHECK (bonus BETWEEN 1 AND 54),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	-- числа хранятся отсортированными; строгое возрастание гарантирует и различность
	CHECK (n1 < n2 AND n2 < n3 AND n3 < n4 AND n4 < n5 AND n5 < n6 AND n6 < n7)
);`

// Open открывает (и при необходимости создаёт) базу. Для тестов допустим ":memory:".
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		// Прагмы через DSN применяются к каждому соединению пула;
		// db.Exec(PRAGMA) задействовал бы только первое соединение.
		dsn += "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("открыть sqlite: %w", err)
	}
	if path == ":memory:" {
		// Каждое соединение получает свою память; ограничиваем пул одним.
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("схема: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const insertCols = `INSERT INTO draws (draw_no, n1, n2, n3, n4, n5, n6, n7, bonus, created_at, updated_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Create добавляет розыгрыш. Повторный номер — ErrDuplicate.
func (s *Store) Create(d Draw) error {
	nums, err := normalize(d.Numbers)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(insertCols,
		d.DrawNo, nums[0], nums[1], nums[2], nums[3], nums[4], nums[5], nums[6], d.Bonus, now, now)
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) {
			switch se.Code() {
			case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqlite3.SQLITE_CONSTRAINT_UNIQUE:
				return ErrDuplicate
			}
		}
		return fmt.Errorf("insert draw: %w", err)
	}
	return nil
}

// normalize проверяет и сортирует семёрку.
func normalize(nums []int) ([]int, error) {
	if len(nums) != 7 {
		return nil, errors.New("нужно ровно 7 чисел")
	}
	out := append([]int(nil), nums...)
	sort.Ints(out)
	return out, nil
}

// List возвращает все розыгрыши по убыванию номера.
func (s *Store) List() ([]Draw, error) {
	rows, err := s.db.Query(`SELECT draw_no, n1, n2, n3, n4, n5, n6, n7, bonus, created_at, updated_at
		FROM draws ORDER BY draw_no DESC`)
	if err != nil {
		return nil, fmt.Errorf("list draws: %w", err)
	}
	defer rows.Close()
	out := []Draw{}
	for rows.Next() {
		var d Draw
		var n1, n2, n3, n4, n5, n6, n7 int
		var created, updated string
		if err := rows.Scan(&d.DrawNo, &n1, &n2, &n3, &n4, &n5, &n6, &n7,
			&d.Bonus, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan draw: %w", err)
		}
		d.Numbers = []int{n1, n2, n3, n4, n5, n6, n7}
		if d.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, fmt.Errorf("created_at: %w", err)
		}
		if d.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
			return nil, fmt.Errorf("updated_at: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// Update заменяет комбинацию розыгрыша drawNo.
func (s *Store) Update(drawNo int64, numbers []int, bonus int) error {
	nums, err := normalize(numbers)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE draws SET n1=?, n2=?, n3=?, n4=?, n5=?, n6=?, n7=?, bonus=?, updated_at=? WHERE draw_no=?`,
		nums[0], nums[1], nums[2], nums[3], nums[4], nums[5], nums[6], bonus,
		time.Now().UTC().Format(time.RFC3339), drawNo)
	if err != nil {
		return fmt.Errorf("update draw: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete удаляет розыгрыш drawNo.
func (s *Store) Delete(drawNo int64) error {
	res, err := s.db.Exec(`DELETE FROM draws WHERE draw_no=?`, drawNo)
	if err != nil {
		return fmt.Errorf("delete draw: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Freq — сколько раз выпал шар n.
type Freq struct {
	N     int `json:"n"`
	Count int `json:"count"`
}

// frequency выполняет запрос вида «номер, количество» с сортировкой
// count DESC, затем номер ASC, и собирает результат в срез.
func (s *Store) frequency(query string) ([]Freq, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("frequency: %w", err)
	}
	defer rows.Close()
	out := []Freq{}
	for rows.Next() {
		var f Freq
		if err := rows.Scan(&f.N, &f.Count); err != nil {
			return nil, fmt.Errorf("scan freq: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

const mainFrequencyQuery = `SELECT n, COUNT(*) c FROM (
	SELECT n1 n FROM draws UNION ALL
	SELECT n2 FROM draws UNION ALL
	SELECT n3 FROM draws UNION ALL
	SELECT n4 FROM draws UNION ALL
	SELECT n5 FROM draws UNION ALL
	SELECT n6 FROM draws UNION ALL
	SELECT n7 FROM draws
) GROUP BY n ORDER BY c DESC, n ASC`

// MainFrequency — сколько раз выпал каждый основной шар (n1..n7).
// Никогда не выпадавшие номера в результат не попадают.
func (s *Store) MainFrequency() ([]Freq, error) {
	return s.frequency(mainFrequencyQuery)
}
