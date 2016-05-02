package gcommerce

import (
	"github.com/fernandez14/go-siftscience"
	"gopkg.in/mgo.v2/bson"

	"errors"
	"time"
)

// Set DI instance
func (this *Order) SetDI(di *Module) {

	this.di = di

	// Setup gateway
	gateway, err := getGateway(this.Gateway)

	if err != nil {
		panic(err)
	}

	this.gateway = gateway
	this.gateway.SetDI(this.di)
	this.gateway.SetOrder(this)
}

func (this *Order) ChangeStatus(name string) {

	database := this.di.Mongo.Database

	is_massdrop := false

	for _, item := range this.Items {

		if related, exists := item.Meta["related"].(string); exists {

			if related == "massdrop_product" {

				is_massdrop = true
			}
		}
	}

	// If there's a massdrop product in order and it goes from awaiting to confirmed then update possible massdrop transaction
	if is_massdrop && this.Status == ORDER_AWAITING && name == ORDER_CONFIRMED {

		var transaction *MassdropTransaction

		err := database.C("gcommerce_massdrop_transactions").Find(bson.M{"customer_id": this.UserId, "type": "interested", "attributes.order_id": this.Id}).One(&transaction)

		if err == nil {

			transaction.SetDI(this.di)
			transaction.CastToReservation()
		}
	}

	status := Status{
		this.Status,
		make(map[string]interface{}),
		this.Updated,
	}

	err := database.C("gcommerce_orders").Update(bson.M{"_id": this.Id}, bson.M{"$set": bson.M{"status": name, "updated_at": time.Now()}, "$push": bson.M{"statuses": status}})

	if err != nil {
		panic(err)
	}
}

func (this *Order) Add(name, description, image string, price float64, q int, meta map[string]interface{}) {

	// Update price based on gateway
	origin_price := price * float64(q)
	gateway_price := this.gateway.ModifyPrice(price) * float64(q)

	this.Items = append(this.Items, Item{name, image, description, gateway_price, origin_price, q, meta})
	this.Total = this.Total + gateway_price
	this.OTotal = this.OTotal + origin_price
}

func (this *Order) Ship(price float64, name string, address *CustomerAddress) {

	this.Shipping = &Shipping{}

	gateway_price := this.gateway.ModifyPrice(price)

	this.Shipping.OPrice = price
	this.Shipping.Price = gateway_price
	this.Shipping.Type = name

	if address != nil {

		this.Shipping.Address = &address.Address

		// Save the address reference in case we need it
		this.Shipping.Meta = map[string]interface{}{
			"related_id": address.Id,
		}

		// Use the address once
		address.UseOnce()
		this.CustomerAdress = address
	} else {

		this.Meta["addressless"] = true
		this.Meta["skip_siftscience"] = true
	}

	this.Total = this.Total + gateway_price
	this.OTotal = this.OTotal + price
}

func (this *Order) GetRelatedAddress() *CustomerAddress {

	if this.CustomerAdress == nil {

		database := this.di.Mongo.Database
		address_id := this.Shipping.Meta["related_id"].(bson.ObjectId)

		var a CustomerAddress

		err := database.C("customer_addresses").Find(bson.M{"_id": address_id}).One(&a)

		if err != nil {
			panic(err)
		}

		this.CustomerAdress = &a
	}

	return this.CustomerAdress
}

func (this *Order) GetTotal() float64 {
	return this.gateway.AdjustPrice(this.Total)
}

func (this *Order) GetOriginalTotal() float64 {
	return this.OTotal
}

func (this *Order) GetGatewayCommision() float64 {
	return this.Total - this.OTotal
}

func (this *Order) GetCustomer() *Customer {

	if this.Customer == nil {

		var c Customer

		database := this.di.Mongo.Database
		err := database.C("customers").FindId(this.UserId).One(&c)

		if err != nil {
			panic(err)
		}

		this.Customer = &c
		this.Customer.SetDI(this.di)
	}

	return this.Customer
}

func (this *Order) Checkout() error {

	// Meta stuff
	meta := this.Meta
	meta["reference"] = this.Reference

	this.gateway.SetMeta(meta)

	// Charge the user
	err := this.gateway.Charge(this.Total)

	return err
}

func (this *Order) Save() error {

	database := this.di.Mongo.Database

	// Global price mutators
	this.Total = this.gateway.AdjustPrice(this.Total)

	// Perform the save of the order once we've got here
	err := database.C("gcommerce_orders").Insert(this)

	if err != nil {
		return errors.New("internal-error")
	}

	_, skip_siftscience := this.Meta["skip_siftscience"]

	if this.Gateway == "stripe" && !skip_siftscience {

		token, exists := this.Meta["token"].(string)

		if exists {

			var items []map[string]interface{}

			customer := this.GetCustomer()
			usr := customer.GetUser()
			caddress := this.GetRelatedAddress()
			micros := int64((this.Total * 100) * 10000)

			// Billing & shipping address
			address := map[string]interface{}{
				"$name":      caddress.Recipient,
				"$phone":     caddress.Phone,
				"$address_1": caddress.Line1(),
				"$address_2": caddress.Line2(),
				"$city":      caddress.Address.City,
				"$region":    caddress.Address.State,
				"$country":   "MX",
				"$zipcode":   caddress.Address.PostalCode,
			}

			products := this.di.Products()

			for _, item := range this.Items {

				item_id, is_valid := item.Meta["related_id"].(bson.ObjectId)

				if is_valid {

					item_micros := int64((item.OPrice * 100) * 10000)
					product, err := products.GetById(item_id)

					if err == nil {

						manufacturer, mexists := product.Attrs["manufacturer"].(string)

						if !mexists {
							manufacturer = "unknown"
						}

						items = append(items, map[string]interface{}{
							"$item_id":       item_id.Hex(),
							"$product_title": item.Name,
							"$price":         item_micros,
							"$currency_code": "MXN",
							"$brand":         manufacturer,
							"$manufacturer":  manufacturer,
							"$category":      product.Category,
							"$quantity":      item.Quantity,
						})
					}
				}
			}

			data := map[string]interface{}{
				"$order_id":        this.Reference,
				"$user_id":         usr.Data().Id.Hex(),
				"$user_email":      usr.Email(),
				"$amount":          micros,
				"$currency_code":   "MXN",
				"$billing_address": address,
				"$payment_methods": []map[string]interface{}{
					{
						"$payment_type":    "$credit_card",
						"$payment_gateway": "$stripe",
						"$stripe_token":    token,
					},
				},
				"$shipping_address":   address,
				"$expedited_shipping": false,
				"$shipping_method":    "$physical",
				"$items":              items,
			}

			err = gosift.Track("$create_order", data)

			if err != nil {
				return errors.New("internal-error")
			}
		}
	}

	return nil
}
