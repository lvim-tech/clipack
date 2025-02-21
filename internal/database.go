package internal

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB() *sql.DB {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	configDir := filepath.Join(home, ".config", "clipack")
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	dbPath := filepath.Join(configDir, "applications.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS applications (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		download_url TEXT,
		install_command TEXT,
		version TEXT,
		config TEXT
	);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Fatalf("%q: %s\n", err, sqlStmt)
	}

	return db
}

func AddInstruction(db *sql.DB, name, downloadURL, installCommand, version, config string) {
	stmt, err := db.Prepare("INSERT INTO applications(name, download_url, install_command, version, config) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(name, downloadURL, installCommand, version, config)
	if err != nil {
		log.Fatal(err)
	}
}

func ListInstructions(db *sql.DB) {
	rows, err := db.Query("SELECT id, name, download_url, install_command, version, config FROM applications")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, downloadURL, installCommand, version, config string
		err = rows.Scan(&id, &name, &downloadURL, &installCommand, &version, &config)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d: %s\n  Download URL: %s\n  Install Command: %s\n  Version: %s\n  Config: %s\n\n", id, name, downloadURL, installCommand, version, config)
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
}

func EditInstruction(db *sql.DB, id int, name, downloadURL, installCommand, version, config string) {
	if name != "" {
		stmt, err := db.Prepare("UPDATE applications SET name = ? WHERE id = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(name, id)
		if err != nil {
			log.Fatal(err)
		}
	}
	if downloadURL != "" {
		stmt, err := db.Prepare("UPDATE applications SET download_url = ? WHERE id = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(downloadURL, id)
		if err != nil {
			log.Fatal(err)
		}
	}
	if installCommand != "" {
		stmt, err := db.Prepare("UPDATE applications SET install_command = ? WHERE id = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(installCommand, id)
		if err != nil {
			log.Fatal(err)
		}
	}
	if version != "" {
		stmt, err := db.Prepare("UPDATE applications SET version = ? WHERE id = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(version, id)
		if err != nil {
			log.Fatal(err)
		}
	}
	if config != "" {
		stmt, err := db.Prepare("UPDATE applications SET config = ? WHERE id = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(config, id)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func ListApplications(db *sql.DB) {
	rows, err := db.Query("SELECT id, name, version FROM applications")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, version string
		err = rows.Scan(&id, &name, &version)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d: %s (version: %s)\n", id, name, version)
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
}
