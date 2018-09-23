package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"os"
	"path/filepath"
)

type Bookshelf struct {
	XMLName xml.Name `xml:"bookshelf" bson:"-"`
	User    User     `xml:"user"`
	Title   string   `xml:"title"`
	Intro   string   `xml:"intro"`
	Books   []Book   `xml:"books>book"`
}

type User struct {
	Id    string `xml:"-" json:"-"`
	Name  string `xml:"name"`
	Email string `xml:"email"`
}

type Book struct {
	Id       int64    `xml:"-" json:"-" bson:"-"`
	Name     string   `xml:"name"`
	Authors  []Author `xml:"authors>author"`
	Publish  Publish  `xml:"publish"`
	Url      string   `xml:"url"`
	Cover    string   `xml:"cover"`
	MyRating int      `xml:"my>rating"`
	MyReview string   `xml:"my>review"`
}

type Publish struct {
	Publisher Publisher `xml:"publisher"`
	Year      int       `xml:"year"`
	Pages     int       `xml:"pages"`
}

type NameAndUrl struct {
	Name string `xml:"name"`
	Url  string `xml:"url"`
}

type Publisher struct {
	NameAndUrl `bson:",inline"`
}

type Author struct {
	Id         int64 `xml:"-" json:"-" bson:"-"`
	NameAndUrl `bson:",inline"`
}

func (b *Book) MyRatingPercent() int {
	return b.MyRating * 20
}

// ---

type BookshelfReader interface {
	get() Bookshelf
}

type BookshelfWriter interface {
	set() Bookshelf
}

// ---

type BookshelfReaderXml struct {
	Filename string
}

func (r *BookshelfReaderXml) get() Bookshelf {
	filePath, err := filepath.Abs(r.Filename)
	if err != nil {
		panic(err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}

	defer file.Close()

	var bookshelf Bookshelf
	if err := xml.NewDecoder(file).Decode(&bookshelf); err != nil {
		panic(err)
	}

	return bookshelf
}

type BookshelfWriterXml struct {
	Filename string
}

func (r *BookshelfWriterXml) set(bookshelf Bookshelf) {
	output, err := xml.MarshalIndent(bookshelf, "", "	")
	if err != nil {
		panic(err)
	}

	file, err := os.Create(r.Filename)
	if err != nil {
		panic(err)
	}

	defer file.Close()

	file.Write([]byte(xml.Header))
	file.Write(output)
}

// ---

type BookshelfReaderSql struct {
	db     *sql.DB
	userId string
}

func (r *BookshelfReaderSql) get() Bookshelf {

	bookshelf := Bookshelf{}
	bookshelf.User = User{}
	stmt, err := r.db.Prepare("SELECT identifier, name, email, title, intro FROM users WHERE identifier = ?")
	checkErr(err)

	rows, err := stmt.Query(r.userId)
	checkErr(err)

	for rows.Next() {
		err = rows.Scan(&bookshelf.User.Id, &bookshelf.User.Name, &bookshelf.User.Email, &bookshelf.Title, &bookshelf.Intro)
		checkErr(err)
	}

	stmt, err = r.db.Prepare("SELECT id, name, url, cover, publisher_name, publisher_url, year, pager, my_rating, my_review FROM books WHERE user = ?")
	checkErr(err)

	rows, err = stmt.Query(r.userId)
	checkErr(err)

	for rows.Next() {
		book := Book{}
		err = rows.Scan(&book.Id, &book.Name, &book.Url, &book.Cover, &book.Publish.Publisher.Name, &book.Publish.Publisher.Url, &book.Publish.Year, &book.Publish.Pages, &book.MyRating, &book.MyReview)
		checkErr(err)

		stmt, err = r.db.Prepare("SELECT authors.id, authors.name, authors.url FROM authors LEFT JOIN books_authors ON author_id = authors.id WHERE book_id = ?")
		checkErr(err)

		rowsAuthors, err := stmt.Query(book.Id)
		checkErr(err)

		for rowsAuthors.Next() {
			author := Author{}
			err = rowsAuthors.Scan(&author.Id, &author.Name, &author.Url)
			checkErr(err)
			book.Authors = append(book.Authors, author)
		}

		bookshelf.Books = append(bookshelf.Books, book)
	}

	return bookshelf
}

type BookshelfWriterSql struct {
	db *sql.DB
}

func (w *BookshelfWriterSql) set(bookshelf Bookshelf) {

	stmt, err := w.db.Prepare("DELETE FROM users WHERE identifier = ?")
	checkErr(err)

	_, err = stmt.Exec(bookshelf.User.Id)
	checkErr(err)

	stmt, err = w.db.Prepare("INSERT INTO users(identifier, name, email, title, intro) values(?,?,?,?,?)")
	checkErr(err)

	_, err = stmt.Exec(bookshelf.User.Id, bookshelf.User.Name, bookshelf.User.Email, bookshelf.Title, bookshelf.Intro)
	checkErr(err)

	authorsUnique := make(map[string]Author)
	for _, book := range bookshelf.Books {
		for _, author := range book.Authors {
			sid := author.Name + author.Url
			if _, has := authorsUnique[sid]; !has {
				stmt, err := w.db.Prepare("INSERT INTO authors(name, user, url) values(?,?,?)")
				checkErr(err)
				result, err := stmt.Exec(author.Name, bookshelf.User.Id, author.Url)
				checkErr(err)
				author.Id, err = result.LastInsertId()
				checkErr(err)

				authorsUnique[sid] = author
			}
		}
	}

	for _, book := range bookshelf.Books {
		stmt, err := w.db.Prepare("INSERT INTO books(user, name, url, cover, publisher_name, publisher_url, year, pager, my_rating, my_review) values(?,?,?,?,?,?,?,?,?,?)")
		checkErr(err)

		res, err := stmt.Exec(bookshelf.User.Id, book.Name, book.Url, book.Cover, book.Publish.Publisher.Name, book.Publish.Publisher.Url, book.Publish.Year, book.Publish.Pages, book.MyRating, book.MyReview)
		checkErr(err)

		bookId, err := res.LastInsertId()
		checkErr(err)

		for _, author := range book.Authors {
			stmt, err = w.db.Prepare("INSERT INTO books_authors(book_id, author_id) values(?,?)")
			checkErr(err)

			_, err = stmt.Exec(bookId, authorsUnique[author.Name+author.Url].Id)
			checkErr(err)
		}
	}
}

func (w *BookshelfWriterSql) initDb() {
	stmt, err := w.db.Prepare("CREATE TABLE users(identifier VARCHAR PRIMARY KEY ASC, name VARCHAR, email VARCHAR, title VARCHAR, intro VARCHAR);")
	checkErr(err)

	stmt, err = w.db.Prepare("CREATE TABLE books(id INTEGER PRIMARY KEY, user VARCHAR, name VARCHAR, url VARCHAR, cover VARCHAR, publisher_name VARCHAR, publisher_url VARCHAR, year INTEGER, pager INTEGER, my_rating INTEGER, my_review VARCHAR, FOREIGN KEY(user) REFERENCES users(identifier) ON DELETE CASCADE ON UPDATE CASCADE);")
	checkErr(err)

	stmt, err = w.db.Prepare("CREATE TABLE authors(id INTEGER PRIMARY KEY, user VARCHAR, name VARCHAR, url VARCHAR, FOREIGN KEY(user) REFERENCES users(identifier) ON DELETE CASCADE ON UPDATE CASCADE);")
	checkErr(err)

	stmt, err = w.db.Prepare("CREATE TABLE books_authors(book_id INTEGER, author_id INTEGER,  PRIMARY KEY(book_id, author_id), FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE ON UPDATE CASCADE, FOREIGN KEY(author_id) REFERENCES authors(id) ON DELETE CASCADE ON UPDATE CASCADE);")
	checkErr(err)

	_, err = stmt.Exec()
	checkErr(err)
}

// ---

type BookshelfWriterMongo struct {
	collection *mongo.Collection
}

func (r *BookshelfWriterMongo) set(bookshelf Bookshelf) {
	_, err := r.collection.InsertOne(context.TODO(), bookshelf)
	checkErr(err)
}

type BookshelfReaderMongo struct {
	collection *mongo.Collection
	userId     string
}

func (r *BookshelfReaderMongo) get() Bookshelf {
	bookshelf := Bookshelf{}
	filter := bson.NewDocument(bson.EC.String("user.id", r.userId))
	err := r.collection.FindOne(context.Background(), filter).Decode(&bookshelf)
	checkErr(err)

	return bookshelf
}

// ---

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
