package controller

import (
	"github.com/fernandez14/spartangeek-blacker/core/user"
	"github.com/fernandez14/spartangeek-blacker/deps"
	"github.com/fernandez14/spartangeek-blacker/modules/store"
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2/bson"

	"sort"
	"strconv"
	"time"
)

func UpsertBLead(c *gin.Context) {
	var form store.BLead
	if err := c.BindJSON(&form); err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	form.Address = c.ClientIP()

	lead, err := store.UpsertBLead(deps.Container, form)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(200, lead)
}

func Leads(c *gin.Context) {
	var (
		limit  int    = 0
		offset int    = 0
		group  string = c.Query("group")
		list   store.Leads
		err    error
	)

	user := c.MustGet("user").(user.User)
	if n, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		offset = n
	}
	if n, err := strconv.Atoi(c.DefaultQuery("limit", "10")); err == nil {
		limit = n
	}

	// After processing input compute output data.
	switch {
	case group == "brandNew":
		list, err = store.FetchLeads(deps.Container, store.NewLeads(limit, offset))
	case group == "nextUp":
		list, err = store.FetchLeads(deps.Container, store.NextUpLeads(deps.Container, limit, offset))
	}

	if err != nil {
		c.JSON(500, err)
		return
	}

	// Compute runtime field UserReaded for each item in the list.
	list = list.HadRead(user.Id)

	c.JSON(200, gin.H{"entities": list.ToMap(), "results": list.IDList()})
}

type StoreAPI struct {
	Store *store.Module `inject:""`
}

// Place an order (public endpoint)
func (self StoreAPI) PlaceOrder(c *gin.Context) {
	var (
		form OrderForm
		ip   string = c.ClientIP()
	)

	if c.BindJSON(&form) == nil {
		if len(form.Ref) == 0 {
			form.Ref = "unknown"
		}

		order := store.OrderModel{
			User: store.OrderUserModel{
				Name:  form.User.Name,
				Email: form.User.Email,
				Phone: form.User.Phone,
				Ip:    ip,
			},
			Content:   form.Content,
			Budget:    form.Budget,
			Currency:  "MXN",
			State:     form.State,
			Games:     form.Games,
			Extra:     form.Extra,
			Usage:     form.Usage,
			BuyDelay:  form.BuyDelay,
			Reference: form.Ref,
		}

		self.Store.CreateOrder(order)

		c.JSON(200, gin.H{"status": "okay", "signed": ip})
	}
}

// Get all orders sorted by convenience
func (self StoreAPI) Orders(c *gin.Context) {
	var limit, offset int = 10, 0

	if n, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		offset = n
	}
	if n, err := strconv.Atoi(c.DefaultQuery("limit", "10")); err == nil {
		limit = n
	}

	user := c.MustGet("user").(user.User)
	search := c.Query("search")
	group := c.Query("group")
	version := c.Query("version")
	orders := self.Store.GetSortedOrders(limit, offset, search, group)
	orders = store.HaveRead(orders, user.Id)

	if version == "v2" {
		list := make([]string, len(orders))
		entities := make(map[string]interface{}, len(orders))

		for index, order := range orders {
			list[index] = order.Id.Hex()
			entities[order.Id.Hex()] = order
		}

		c.JSON(200, gin.H{"entities": entities, "results": list})
		return
	}

	c.JSON(200, orders)
}

func (s StoreAPI) OrdersAggregate(c *gin.Context) {

	tags := s.Store.GetOrdersAggregation()

	c.JSON(200, tags)
}

// REST handler for getting one order
func (self StoreAPI) One(c *gin.Context) {
	user := c.MustGet("user").(user.User)
	id := bson.ObjectIdHex(c.Param("id"))

	// Fetch order using provided ID
	order, err := self.Store.Order(id)
	if err != nil {
		c.JSON(404, gin.H{"status": "error", "message": "Order not found."})
		return
	}

	// Mark as readed
	order.Touch(user.Id)

	// Load assets
	order.LoadAssets()
	order.LoadDuplicates()
	order.LoadActivities()
	order.LoadBLead()

	data := store.HasBeenRead(*order.Data(), user.Id)
	data.RelatedUsers = order.MatchUsers()
	sort.Sort(data.Messages)

	c.JSON(200, data)
}

func (self StoreAPI) Ignore(c *gin.Context) {

	id := c.Param("id")

	if bson.IsObjectIdHex(id) == false {
		c.JSON(400, gin.H{"status": "error", "message": "Can't perform action. Invalid id."})
		return
	}

	order, err := self.Store.Order(bson.ObjectIdHex(id))

	if err != nil {
		c.JSON(404, gin.H{"status": "error", "message": "Order not found."})
		return
	}

	// Mark as ignored
	order.Ignore()

	c.JSON(200, gin.H{"status": "okay"})
}

// Answer with text to an order
func (self *StoreAPI) Answer(c *gin.Context) {
	var form OrderAnswerForm
	order_id := c.Param("id")

	if bson.IsObjectIdHex(order_id) == false {
		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(order_id)
	if c.BindJSON(&form) == nil {

		lead, err := store.FindLead(deps.Container, id)
		if err != nil {
			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
			return
		}

		id, err := lead.Reply(form.Content, form.Type)
		if err != nil {
			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
			return
		}

		c.JSON(200, gin.H{"status": "okay", "id": id})
	}
}

// Push tag to an order
func (self *StoreAPI) Tag(c *gin.Context) {
	var form OrderTagForm

	order_id := c.Param("id")

	if bson.IsObjectIdHex(order_id) == false {

		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(order_id)

	if c.BindJSON(&form) == nil {

		order, err := self.Store.Order(id)

		if err == nil {

			order.PushTag(form.Name)

			c.JSON(200, gin.H{"status": "okay"})
		} else {

			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		}
	}
}

func (self *StoreAPI) DeleteTag(c *gin.Context) {
	var f OrderTagForm

	oid := c.Param("id")
	if !bson.IsObjectIdHex(oid) {
		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(oid)
	if c.BindJSON(&f) == nil {
		order, err := self.Store.Order(id)
		if err != nil {
			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		}

		_ = order.DeleteTag(f.Name)
		c.JSON(200, gin.H{"status": "okay"})
	}
}

func (self *StoreAPI) Trust(c *gin.Context) {

	var form TrustForm

	order_id := c.Param("id")

	if bson.IsObjectIdHex(order_id) == false {
		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(order_id)

	if c.BindJSON(&form) == nil {
		order, err := self.Store.Order(id)

		if err == nil {
			order.SetTrusted(form.Trusted)
			c.JSON(200, gin.H{"status": "okay"})
		} else {
			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		}
	}
}

func (api *StoreAPI) Unread(c *gin.Context) {
	id := bson.ObjectIdHex(c.Param("id"))
	user := c.MustGet("user").(user.User)

	// Fetch order using provided ID
	order, err := api.Store.Order(id)
	if err != nil {
		c.JSON(404, gin.H{"status": "error", "message": "Order not found."})
		return
	}

	order.Unread(user.Id)

	c.JSON(200, gin.H{"status": "okay"})
}

func (self *StoreAPI) Favorite(c *gin.Context) {

	var form FavoriteForm

	order_id := c.Param("id")

	if bson.IsObjectIdHex(order_id) == false {
		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(order_id)

	if c.BindJSON(&form) == nil {
		order, err := self.Store.Order(id)

		if err == nil {
			order.SetFlag("favorite", form.Favorite)
			c.JSON(200, gin.H{"status": "okay"})
		} else {
			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		}
	}
}

// Update order stage
func (self *StoreAPI) Stage(c *gin.Context) {

	var form OrderStageForm

	order_id := c.Param("id")

	if bson.IsObjectIdHex(order_id) == false {

		c.JSON(400, gin.H{"message": "Invalid request, id not valid.", "status": "error"})
		return
	}

	id := bson.ObjectIdHex(order_id)

	if c.BindJSON(&form) == nil {

		order, err := self.Store.Order(id)

		if err == nil {

			order.Stage(form.Name)

			c.JSON(200, gin.H{"status": "okay"})
		} else {

			c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		}
	}
}

// Assign a new activity related to a lead.
func (self *StoreAPI) Activity(c *gin.Context) {
	var form store.Activity
	err := c.BindJSON(&form)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	id := bson.ObjectIdHex(c.Param("id"))
	lead, err := store.FindLead(deps.Container, id)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	activity, err := store.AssignActivity(deps.Container, *lead, form)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "okay", "activity": activity})
}

func (self *StoreAPI) Activities(c *gin.Context) {
	var limit, offset int = 10, 0
	var dates []time.Time

	if n, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		offset = n
	}

	if n, err := strconv.Atoi(c.DefaultQuery("limit", "10")); err == nil {
		limit = n
	}

	if r, active := c.GetQueryArray("dates"); active {
		for _, date := range r {
			datetime, err := time.Parse(time.RFC3339, date)
			if err == nil {
				dates = append(dates, datetime)
			}
		}
	}

	activities, err := store.FindActivities(deps.Container, dates, offset, limit)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "message": err.Error()})
		return
	}

	leads, err := store.FetchLeads(deps.Container, activities.LeadsQuery())

	c.JSON(200, gin.H{"leads": leads.ToMap(), "activities": activities.ToMap()})
}

type OrderForm struct {
	User     OrderUserForm `json:"user" binding:"required"`
	Content  string        `json:"content"`
	Budget   int           `json:"budget" binding:"required"`
	BuyDelay int           `json:"buydelay" binding:"required"`
	State    string        `json:"estado" binding:"required"`
	Usage    string        `json:"usage"`
	Ref      string        `json:"reference"`
	Games    []string      `json:"games"`
	Extra    []string      `json:"extra"`
}

type OrderUserForm struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required"`
	Phone string `json:"phone" binding:"required"`
}

type OrderTagForm struct {
	Name string `json:"name" binding:"required"`
}

type TrustForm struct {
	Trusted bool `json:"trusted"`
}

type FavoriteForm struct {
	Favorite bool `json:"favorite"`
}

type OrderStageForm struct {
	Name string `json:"name" binding:"required"`
}

type OrderActivityForm struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Due         string `json:"due_at" binding:"required"`
}

type OrderAnswerForm struct {
	Content string `json:"content" binding:"required"`
	Type    string `json:"type" binding:"required"`
}
