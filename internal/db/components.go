package db

import (
	"time"

	"github.com/quay/release-readiness/internal/model"
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

func (d *DB) EnsureComponent(name string) (*model.Component, error) {
	comp, err := d.GetComponentByName(name)
	if err == nil {
		return comp, nil
	}
	return d.CreateComponent(name, "")
}
