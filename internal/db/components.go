package db

import (
	"fmt"
	"time"

	"github.com/quay/build-dashboard/internal/model"
)

func (d *DB) ListComponents() ([]model.Component, error) {
	rows, err := d.Query(`SELECT id, name, description, created_at FROM components ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var components []model.Component
	for rows.Next() {
		var c model.Component
		var ts string
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &ts); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, ts)

		suites, err := d.listComponentSuites(c.ID)
		if err != nil {
			return nil, err
		}
		c.Suites = suites
		components = append(components, c)
	}
	return components, rows.Err()
}

func (d *DB) CreateComponent(name, description string) (*model.Component, error) {
	res, err := d.Exec(`INSERT INTO components (name, description) VALUES (?, ?)`, name, description)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Component{ID: id, Name: name, Description: description, CreatedAt: time.Now().UTC()}, nil
}

func (d *DB) GetComponentByName(name string) (*model.Component, error) {
	var c model.Component
	var ts string
	err := d.QueryRow(`SELECT id, name, description, created_at FROM components WHERE name = ?`, name).
		Scan(&c.ID, &c.Name, &c.Description, &ts)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, ts)
	return &c, nil
}

func (d *DB) ListSuites() ([]model.Suite, error) {
	rows, err := d.Query(`SELECT id, name, description, created_at FROM test_suites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []model.Suite
	for rows.Next() {
		var s model.Suite
		var ts string
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &ts); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

func (d *DB) CreateSuite(name, description string) (*model.Suite, error) {
	res, err := d.Exec(`INSERT INTO test_suites (name, description) VALUES (?, ?)`, name, description)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Suite{ID: id, Name: name, Description: description, CreatedAt: time.Now().UTC()}, nil
}

func (d *DB) GetSuiteByName(name string) (*model.Suite, error) {
	var s model.Suite
	var ts string
	err := d.QueryRow(`SELECT id, name, description, created_at FROM test_suites WHERE name = ?`, name).
		Scan(&s.ID, &s.Name, &s.Description, &ts)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
	return &s, nil
}

func (d *DB) MapSuiteToComponent(componentName, suiteName string, required bool) error {
	comp, err := d.GetComponentByName(componentName)
	if err != nil {
		return fmt.Errorf("component %q not found: %w", componentName, err)
	}
	suite, err := d.GetSuiteByName(suiteName)
	if err != nil {
		return fmt.Errorf("suite %q not found: %w", suiteName, err)
	}
	reqInt := 0
	if required {
		reqInt = 1
	}
	_, err = d.Exec(
		`INSERT OR REPLACE INTO component_suites (component_id, suite_id, required) VALUES (?, ?, ?)`,
		comp.ID, suite.ID, reqInt,
	)
	return err
}

func (d *DB) UnmapSuiteFromComponent(componentName, suiteName string) error {
	comp, err := d.GetComponentByName(componentName)
	if err != nil {
		return err
	}
	suite, err := d.GetSuiteByName(suiteName)
	if err != nil {
		return err
	}
	_, err = d.Exec(`DELETE FROM component_suites WHERE component_id = ? AND suite_id = ?`, comp.ID, suite.ID)
	return err
}

func (d *DB) listComponentSuites(componentID int64) ([]model.Suite, error) {
	rows, err := d.Query(`
		SELECT ts.id, ts.name, ts.description, ts.created_at, cs.required
		FROM test_suites ts
		JOIN component_suites cs ON cs.suite_id = ts.id
		WHERE cs.component_id = ?
		ORDER BY ts.name`, componentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []model.Suite
	for rows.Next() {
		var s model.Suite
		var ts string
		var req int
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &ts, &req); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		s.Required = req == 1
		suites = append(suites, s)
	}
	return suites, rows.Err()
}
