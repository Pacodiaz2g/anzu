package cart

import (
	"encoding/json"
	"github.com/gin-gonic/contrib/sessions"
	"log"
)

type GinGonicSession struct {
	Session sessions.Session
}

func (gcs GinGonicSession) Restore() (map[string]*CartItem, error) {

	var list map[string]*CartItem

	session := gcs.Session
	data := session.Get("cart")

	if data == nil {

		log.Printf("Carrito vacio \n")

		list = make(map[string]*CartItem)

		return list, nil
	} else {

		encoded := data.(string)

		log.Printf("%v\n", encoded)

		if err := json.Unmarshal([]byte(encoded), &list); err != nil {

			return list, err
		}

		return list, nil
	}
}

func (gcs GinGonicSession) Save(data map[string]*CartItem) error {

	encoded, err := json.Marshal(data)

	if err != nil {
		return err
	}

	session := gcs.Session

	session.Set("cart", string(encoded))
	session.Save()

	return nil
}
