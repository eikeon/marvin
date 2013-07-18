package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"go/build"
	"html/template"
	"io"
	"log"
	"net/http"
	"path"

	"code.google.com/p/go.net/websocket"
	"github.com/eikeon/marvin"
)

var Root string
var site *template.Template
var templates = make(map[string]*template.Template)

func init() {
	if p, err := build.Default.Import("github.com/eikeon/marvin/marvin", "", build.FindOnly); err == nil {
		Root = p.Dir
	} else {
		log.Println("WARNING: could not import package:", err)
	}
}

func getTemplate(name string) *template.Template {
	if t, ok := templates[name]; ok {
		return t
	} else {
		if site == nil {
			site = template.Must(template.ParseFiles(path.Join(Root, "templates/site.html")))
		}
		t, err := site.Clone()
		if err != nil {
			log.Fatal("cloning site: ", err)
		}
		t = template.Must(t.ParseFiles(path.Join(Root, name)))
		templates[name] = t
		return t
	}
}

type View interface {
	Prefix() string
	Name() string
	Match(req *http.Request) bool
	Data(req *http.Request) Data
}

type Data map[string]interface{}

type view struct {
	prefix, name string
	data         Data
}

func (v *view) Prefix() string {
	return v.prefix
}

func (v *view) Name() string {
	return v.name
}

func (v *view) Match(req *http.Request) bool {
	return req.URL.Path == v.Prefix()
}

func (v *view) Data(req *http.Request) Data {
	if v.data == nil {
		v.data = make(Data)
	}
	v.data["Title"] = v.Name()
	return v.data
}

func add(view View) {
	t := getTemplate("templates/" + view.Name() + ".html")
	http.HandleFunc(view.Prefix(), func(w http.ResponseWriter, req *http.Request) {
		var d Data
		if view.Match(req) {
			d = view.Data(req)
		} else {
			w.Header().Set("Cache-Control", "max-age=10, must-revalidate")
			w.WriteHeader(http.StatusNotFound)
		}
		var bw bytes.Buffer
		h := md5.New()
		mw := io.MultiWriter(&bw, h)
		err := t.ExecuteTemplate(mw, "html", d)
		if err == nil {
			w.Header().Set("ETag", fmt.Sprintf(`"%x"`, h.Sum(nil)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", bw.Len()))
			w.Write(bw.Bytes())
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

type message map[string]interface{}

func StateServer(marvin *marvin.Marvin) websocket.Handler {
	return func(ws *websocket.Conn) {
		go func() {
			for {
				var msg message
				if err := websocket.JSON.Receive(ws, &msg); err == nil {
					if msg["action"] == "updateSwitch" {
						marvin.Switch[msg["name"].(string)] = msg["value"].(bool)
						marvin.StateChanged()
					} else if msg["action"] == "setHue" {
						marvin.Hue.Set(msg["address"].(string), msg["value"])
						marvin.StateChanged()
					} else {
						log.Printf("ignoring: %#v\n", msg)
					}
				} else {
					log.Println("State Websocket receive err:", err)
					return
				}
			}
		}()
		for {
			if err := websocket.JSON.Send(ws, marvin); err != nil {
				log.Println("State Websocket send err:", err)
				return
			}
			marvin.WaitStateChanged()
		}
	}
}

func AddHandlers(marvin *marvin.Marvin) {
	add(&view{prefix: "/", name: "home", data: Data{"Marvin": marvin}})
	add(&view{prefix: "/hue/", name: "hue", data: Data{"Marvin": marvin}})
	add(&view{prefix: "/schedule/", name: "schedule", data: Data{"Marvin": marvin}})

	fs := http.FileServer(http.Dir(path.Join(Root, "static/")))
	http.Handle("/bootstrap/", fs)
	http.Handle("/jquery/", fs)
	http.Handle("/js/", fs)

	http.HandleFunc("/post", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			if err := req.ParseForm(); err == nil {
				name, ok := req.Form["do_transition"]
				if ok {
					marvin.Do(name[0])
				}
			}
			// TODO: write a response
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/activities/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			if err := req.ParseForm(); err == nil {
				source, sok := req.Form["sourceActivity"]
				target, dok := req.Form["targetActivity"]
				log.Println("s:", source, "d:", target)
				if sok {
					s := marvin.GetActivity(source[0])
					if s != nil && dok {
						s.Next[target[0]] = true
					}
				}
				if dok {
					marvin.GetActivity(target[0])
					marvin.Activity = target[0]
					marvin.Do(target[0])
					marvin.StateChanged()
				}
			}
			// TODO: write a response
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	http.Handle("/state", websocket.Handler(StateServer(marvin)))
}
