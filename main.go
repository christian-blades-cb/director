package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/boltdb/bolt"
	"github.com/jessevdk/go-flags"
	"github.com/zenazn/goji/web"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

func main() {
	var opts struct {
		RedirectorBinding      string `long:"redirector-bind" default:":8000" description:"ip:port to bind the redirector service" env:"REDIRECTOR_BIND"`
		RedirectorReadTimeout  int64  `long:"redirector-readtimeout" default:"5" description:"number of seconds to wait for request before closing the connection" env:"REDIRECTOR_READTIMEOUT"`
		RedirectorWriteTimeout int64  `long:"redirector-writetimeout" default:"2" description:"number of seconds to wait for a response before closing the connection" env:"REDIRECTOR_WRITETIMEOUT"`

		AdminBinding string `long:"admin-bind" default:"8888" description:"ip:port to bind the admin interfact" env:"ADMIN_BIND"`

		StemDBPath string `long:"db-path" default:"stems.db" description:"path to the stems database file" env:"DB_PATH"`
	}

	if _, err := flags.Parse(&opts); err != nil {
		log.Fatal("cannot parse args")
	}

	stemdb := NewStemDB(opts.StemDBPath, "stems")
	if err := stemdb.Open(); err != nil {
		log.WithField("error", err).Fatal("cannot open stem database")
	}
	defer stemdb.Close()
	if err := stemdb.InitializeDatabase(); err != nil {
		log.WithField("error", err).Warn(err)
	}

	redirectorMux := http.NewServeMux()
	redirectorMux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if found, dest := stemdb.GetDestination(req.URL.Path[1:]); found {
			http.Redirect(w, req, dest, 302)
		} else {
			http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
		}
	})
	redirectServer := &http.Server{
		Addr:         opts.RedirectorBinding,
		Handler:      redirectorMux,
		ReadTimeout:  time.Duration(opts.RedirectorReadTimeout) * time.Second,
		WriteTimeout: time.Duration(opts.RedirectorWriteTimeout) * time.Second,
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		log.WithField("addr", opts.RedirectorBinding).Info("Redirector up")
		redirectServer.ListenAndServe()
		log.Info("Redirector down")
		wg.Done()
	}(&wg)

	adminMux := web.New()
	adminMux.Get("/stems", func(c web.C, w http.ResponseWriter, r *http.Request) {
		stemChan := make(chan StemDest)
		go stemdb.GetStems(stemChan)
		stemMap := map[string]string{}
		for sd := range stemChan {
			stemMap[sd.Stem] = sd.Dest
		}
		w.Header().Add("Content-type", "application/json; charset=utf-8")
		jsonWriter := json.NewEncoder(w)
		jsonWriter.Encode(stemMap)
	})
	adminMux.Put("/stems/*", func(c web.C, w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-type", "application/json; charset=utf-8")

		destBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.WithField("error", err).Error("error reading body")
			http.Error(w, "Internal Error", http.StatusInternalServerError)
		}
		defer r.Body.Close()
		dest := fmt.Sprintf("%s", destBytes)
		dest = strings.TrimSpace(dest)
		if len(dest) < 4 {
			http.Error(w, "body must contain destination URL", http.StatusNotAcceptable)
			return
		}

		stemdb.SetDestination(c.URLParams["*"][1:], dest)
		http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
	})
	adminServer := &http.Server{
		Addr:    opts.AdminBinding,
		Handler: adminMux,
	}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		log.WithField("addr", opts.AdminBinding).Info("Admin server up")
		adminServer.ListenAndServe()
		log.Info("Admin server shut down")
		wg.Done()
	}(&wg)

	wg.Wait()
}

type StemDB struct {
	Path       string
	BucketName string
	db         *bolt.DB
}

func NewStemDB(path, bucket string) *StemDB {
	return &StemDB{
		Path:       path,
		BucketName: bucket,
	}
}

func (s *StemDB) Open() error {
	db, err := bolt.Open(s.Path, 0600, nil)
	s.db = db
	return err
}

func (s *StemDB) Close() error {
	return s.db.Close()
}

func (s *StemDB) InitializeDatabase() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("stems"))
		return err
	})
}

func (s *StemDB) GetDestination(stem string) (exists bool, dest string) {
	s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(s.BucketName))
		destBytes := bucket.Get([]byte(stem))
		if destBytes == nil {
			exists = false
			dest = ""
		} else {
			exists = true
			dest = fmt.Sprintf("%s", destBytes)
		}
		return nil
	})

	return
}

func (s *StemDB) SetDestination(stem, dest string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(s.BucketName))
		return bucket.Put([]byte(stem), []byte(dest))
	})
}

type StemDest struct {
	Stem string
	Dest string
}

func (s *StemDB) GetStems(stemChannel chan<- StemDest) error {
	return s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(s.BucketName))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			stemChannel <- StemDest{
				Stem: fmt.Sprintf("%s", k),
				Dest: fmt.Sprintf("%s", v),
			}
		}
		close(stemChannel)

		return nil
	})
}
