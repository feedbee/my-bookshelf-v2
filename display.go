package main

import (
	"html/template"
	"net/http"
		"path/filepath"
	"os"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type BooksPageData struct {
	Bookshelf Bookshelf

	CoversDir string
	ImgDir string
	CssDir string
}

func main() {
	tmpl := template.Must(template.ParseFiles("layout.html"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		br := BookshelfReaderXml{
			Filename: "public/data/feedbee.xml",
		}
		bs := br.get()

		data := BooksPageData{
			Bookshelf: bs,
			CoversDir: "s/data/covers",
			ImgDir: "s/img",
			CssDir: "s/css",
		}
		err := tmpl.Execute(w, data)
		if err != nil {
			panic(err)
		}
	})

	absolutePath, err := filepath.Abs(filepath.Dir(os.Args[0]) + "/public/")
	if err != nil {
		panic(err)
	}
	http.Handle("/s/", http.StripPrefix("/s/", http.FileServer(http.Dir(absolutePath))))

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		br := BookshelfReaderXml{
			Filename: "public/data/feedbee.xml",
		}
		bw := BookshelfWriterXml{
			Filename: "public/data/feedbee-test.xml",
		}
		bw.set(br.get())
	})

	http.HandleFunc("/save-to-sqlite", func(w http.ResponseWriter, r *http.Request) {
		br := BookshelfReaderXml{
			Filename: "public/data/feedbee.xml",
		}
		db, err := sql.Open("sqlite3", "./db.sqlite")
		checkErr(err)
		bw := BookshelfWriterSql{
			db: db,
		}
		bs := br.get()
		bs.User.Id = "feedbee"
		bw.set(bs)
	})

	http.HandleFunc("/save-to-xml", func(w http.ResponseWriter, r *http.Request) {
		db, err := sql.Open("sqlite3", "./db.sqlite")
		checkErr(err)
		br := BookshelfReaderSql{
			db: db,
			userId: "feedbee",
		}

		bw := BookshelfWriterXml{
			Filename: "public/data/feedbee-from-sql.xml",
		}
		bs := br.get()
		bw.set(bs)
	})

	http.ListenAndServe(":8080", nil)
}