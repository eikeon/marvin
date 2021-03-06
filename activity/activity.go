package activity

import (
	"encoding/json"
	"log"
	"path"
	"runtime"
	"strings"

	"github.com/nogiushi/marvin/nog"
)

var Root = ""

func init() {
	_, filename, _, _ := runtime.Caller(0)
	Root = path.Dir(filename)
}

type activity struct {
	Name string
	Next map[string]bool
}

type Activity struct {
	Activities  map[string]*activity
	Activity    string
	Switch      map[string]bool
	Transitions map[string]struct {
		Switch   map[string]bool
		Commands []struct {
			Address string
			State   string
		}
	}
}

func (m *Activity) GetActivity(name string) *activity {
	if name != "" {
		a, ok := m.Activities[name]
		if !ok {
			a = &activity{name, map[string]bool{}}
			m.Activities[name] = a
		}
		return a
	} else {
		return nil
	}
}

func (m *Activity) UpdateActivity(name string) {
	s := m.GetActivity(m.Activity)
	if s != nil {
		s.Next[name] = true
	}
	m.GetActivity(name)
	m.Activity = name
}

func Handler(in <-chan nog.Message, out chan<- nog.Message) {
	out <- nog.Message{What: "started"}
	a := &Activity{}

	go func() {
		out <- nog.Template("activity")
	}()

	for m := range in {
		if m.Why == "statechanged" {
			dec := json.NewDecoder(strings.NewReader(m.What))
			if err := dec.Decode(a); err != nil {
				log.Println("activity decode err:", err)
			}
		} else {
			const IAM = "I am "
			what := m.What
			if strings.HasPrefix(m.What, IAM) {
				what = m.What[len(IAM):]
				a.UpdateActivity(what)
				if what, err := json.Marshal(a); err == nil {
					out <- nog.Message{What: string(what), Why: "statechanged"}
				} else {
					log.Println("StateChanged err:", err)
				}

				out <- nog.Message{What: "do " + what}
			}
		}
	}
	out <- nog.Message{What: "stopped"}
	close(out)

}
