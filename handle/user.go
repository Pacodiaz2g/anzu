package handle

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/fernandez14/spartangeek-blacker/mongo"
	"github.com/fernandez14/spartangeek-blacker/model"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/mrvdot/golang-utils"
	"gopkg.in/mgo.v2/bson"
	"time"
	"log"
	"sort"
)

type UserAPI struct {
	DataService *mongo.Service `inject:""`
}

func (di *UserAPI) UserSubscribe(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

	var register model.UserSubscribeForm

	if c.BindWith(&register, binding.JSON) == nil {

		subscribe := &model.UserSubscribe{
			Category: register.Category,
			Email: register.Email,
		}

		err := database.C("subscribes").Insert(subscribe)

		if err != nil {
			panic(err)
		}
		c.JSON(200, gin.H{"status": "okay"})
	}
}

func (di *UserAPI) UserGetByToken(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

	// Users collection
	collection := database.C("users")

	// Get user by token
	user_token := model.UserToken{}
	token := c.Request.Header.Get("Auth-Token")

	// Try to fetch the user using token header though
	err := database.C("tokens").Find(bson.M{"token": token}).One(&user_token)

	if err == nil {

		user := model.User{}
		err = collection.Find(bson.M{"_id": user_token.UserId}).One(&user)

		if err == nil {
			
			// Get the user notifications
			notifications, err := database.C("notifications").Find(bson.M{"user_id": user.Id, "seen": false}).Count()
			
			if err !=  nil {
				panic(err)
			}
			
			user.Notifications = notifications
			
			// Alright, go back and send the user info
			c.JSON(200, user)

			return
		}
	}

	c.JSON(401, gin.H{"error": "Could find user by token...", "status": 103})
}

func (di *UserAPI) UserGetToken(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

	// Get the query parameters
	qs := c.Request.URL.Query()

	// Get the email or the username or the id and its password
	email, password := qs.Get("email"), qs.Get("password")

	collection := database.C("users")

	user := model.User{}

	// Try to fetch the user using email param though
	err := collection.Find(bson.M{"email": email}).One(&user)

	if err != nil {

		c.JSON(400, gin.H{"status": "error", "message": "Couldnt found user with that email", "code": 400})
		return
	}

	// Incorrect password
	password_encrypted := []byte(password)
	sha256 := sha256.New()
	sha256.Write(password_encrypted)
	md := sha256.Sum(nil)
	hash := hex.EncodeToString(md)

	if user.Password != hash {

		c.JSON(400, gin.H{"status": "error", "message": "Credentials are not correct", "code": 400})
		return
	}

	// Generate user token
	uuid := uuid.New()
	token := &model.UserToken{
		UserId:  user.Id,
		Token:   uuid,
		Closed:  false,
		Created: time.Now(),
		Updated: time.Now(),
	}

	err = database.C("tokens").Insert(token)

	c.JSON(200, token)
}

func (di *UserAPI) UserGetTokenFacebook(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

	var facebook map[string]interface{}

	// Bind to map
	c.BindWith(&facebook, binding.JSON)

	facebook_id := facebook["id"]

	// Validate the facebook id
	if facebook_id == "" {

		c.JSON(401, gin.H{"error": "Invalid oAuth facebook token...", "status": 105})
		return
	}

	collection := database.C("users")
	user := model.User{}

	// Try to fetch the user using the facebook id param though
	err := collection.Find(bson.M{"facebook.id": facebook_id}).One(&user)

	// Create a new user
	if err != nil {

		var facebook_first_name, facebook_last_name, facebook_email string

		username := facebook["first_name"].(string) + " " + facebook["last_name"].(string)
		id := bson.NewObjectId()

		// Ensure the facebook data is alright
		if _, ok := facebook["first_name"]; ok {

			facebook_first_name = facebook["first_name"].(string)
		} else {

			facebook_first_name = ""
		}

		if _, ok := facebook["last_name"]; ok {

			facebook_last_name = facebook["last_name"].(string)
		} else {

			facebook_last_name = ""
		}

		if _, ok := facebook["email"]; ok {

			facebook_email = facebook["email"].(string)
		} else {

			facebook_email = ""
		}

		user := &model.User{
			Id:          id,
			FirstName:   facebook_first_name,
			LastName:    facebook_last_name,
			UserName:    utils.GenerateSlug(username),
			Password:    "",
			Email:       facebook_email,
			Roles:       make([]string, 0),
			Permissions: make([]string, 0),
			Description: "",
			Facebook:    facebook,
			Created:     time.Now(),
			Updated:     time.Now(),
		}

		err = database.C("users").Insert(user)

		if err != nil {

			c.JSON(500, gin.H{"error": "Could create user using facebook oAuth...", "status": 106})
			return
		}
		
		err = database.C("counters").Insert(model.Counter{
			UserId: id,
			Counters: map[string]model.PostCounter{
				"news": model.PostCounter{
					Counter: 0,
					Updated: time.Now(),
				},
			},
		})

		// Generate user token
		uuid := uuid.New()
		token := &model.UserToken{
			UserId:  id,
			Token:   uuid,
			Created: time.Now(),
			Updated: time.Now(),
		}

		err = database.C("tokens").Insert(token)

		if err != nil {

			panic(err)
		}

		c.JSON(200, token)

	} else {

		// Generate user token
		uuid := uuid.New()
		token := &model.UserToken{
			UserId:  user.Id,
			Token:   uuid,
			Closed:  false,
			Created: time.Now(),
			Updated: time.Now(),
		}

		err = database.C("tokens").Insert(token)

		if err != nil {

			panic(err)
		}

		c.JSON(200, token)
	}
}

func (di *UserAPI) UserUpdateProfile(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

	// Users collection
	users_collection := database.C("users")

	// Get user by token
	user_token := model.UserToken{}
	token := c.Request.Header.Get("Auth-Token")

	// Try to fetch the user using token header though
	err := database.C("tokens").Find(bson.M{"token": token}).One(&user_token)

	if err == nil {

		user := model.User{}
		err = users_collection.Find(bson.M{"_id": user_token.UserId}).One(&user)

		if err == nil {

			var profileUpdate model.UserProfileForm

			if c.BindWith(&profileUpdate, binding.JSON) != nil {

				set := bson.M{}
				
				if profileUpdate.Biography != "" {
					
					set["profile.bio"] = profileUpdate.Biography
				}
				
				if profileUpdate.UserName != "" {
					
					// Check whether user exists
					count, _ := database.C("users").Find(bson.M{"username": profileUpdate.UserName}).Count()
					
					if count == 0 {
						
						set["username"] = profileUpdate.UserName
					}
				}
				
				if profileUpdate.Country != "" {
					
					set["profile.country"] = profileUpdate.Country
				}
				
				if profileUpdate.FavouriteGame != "" {
					
					set["profile.favourite_game"] = profileUpdate.FavouriteGame
				}
				
				if profileUpdate.Microsoft != "" {
					
					set["profile.microsoft"] = profileUpdate.Microsoft
				}
				
				set["updated_at"] = time.Now()
				
				log.Printf("%v", set)
				log.Printf("%v", profileUpdate)

				// Update the user profile with some godness
				users_collection.Update(bson.M{"_id": user.Id}, bson.M{"$set": set})

				c.JSON(200, gin.H{"message": "okay", "status": "okay", "code": 200})
				return
			}
		}
	}
}

func (di *UserAPI) UserRegisterAction(c *gin.Context) {

	// Get the database interface from the DI
	database := di.DataService.Database

    var registerAction model.UserRegisterForm

    if c.BindWith(&registerAction, binding.JSON) != nil {

        // Check if already registered
        email_exists, _ := database.C("users").Find(bson.M{"email": registerAction.Email}).Count()

        if email_exists > 0 {

            // Only one account per email
            c.JSON(400, gin.H{"status": "error", "message": "User already registered", "code": 470})
            return
        }

        user_exists, _ := database.C("users").Find(bson.M{"username": registerAction.UserName}).Count()

        if user_exists > 0 {

            // Username busy
            c.JSON(400, gin.H{"status": "error", "message": "User already registered", "code": 471})
            return
        }

        // Encode password using sha
        password_encrypted := []byte(registerAction.Password)
    	sha256 := sha256.New()
    	sha256.Write(password_encrypted)
    	md := sha256.Sum(nil)
    	hash := hex.EncodeToString(md)

	    // Profile default data
	    profile := gin.H{
	        "step": 0,
	        "ranking": 0,
	        "country": "México",
	        "posts": 0,
	        "followers": 0,
	        "show_email": false,
	        "favourite_game": "-",
	        "microsoft": "-",
	        "bio": "Just another spartan geek",
	    }
		
		id := bson.NewObjectId()
		
        user := &model.User{
        	Id: id,
            FirstName: "",
            LastName: "",
            UserName: registerAction.UserName,
            Password: hash,
            Email: registerAction.Email,
            Roles: []string{"registered"},
            Permissions: make([]string, 0),
            Description: "",
            Profile: profile,
            Stats: model.UserStats{
                Saw: 0,
            },
            Created: time.Now(),
            Updated: time.Now(),
        }
        
        err := database.C("users").Insert(user)

        if err != nil {
            panic(err)
        }
        
        err = database.C("counters").Insert(model.Counter{
			UserId: id,
			Counters: map[string]model.PostCounter{
				"news": model.PostCounter{
					Counter: 0,
					Updated: time.Now(),
				},
			},
		})
        
        // Send a confirmation email

        // Finished creating the post
		c.JSON(200, gin.H{"status": "okay", "code": 200})
		return
    }
    
    // Couldn't process the request though
    c.JSON(400, gin.H{"status": "error", "message": "Missing information to process the request", "code": 400})
}

func (di *UserAPI) UserInvolvedFeedGet(c *gin.Context) {
	
	// Get the database interface from the DI
	database := di.DataService.Database

	var user_posts []model.Post
	var commented_posts []model.Post
	var activity = make([]model.UserActivity, 0)
	
	 // Check whether auth or not
	user_token := model.UserToken{}
	token := c.Request.Header.Get("Auth-Token")

	if token != "" {

    	// Try to fetch the user using token header though
    	err := database.C("tokens").Find(bson.M{"token": token}).One(&user_token)
    
    	if err == nil {
    		
    		var user model.User
    		
    		// Get the current user
    		err := database.C("users").Find(bson.M{"_id": user_token.UserId}).One(&user)
    		
    		if err != nil {
    			panic(err)
    		}
    		
    		// Get the user owned posts
			err = database.C("posts").Find(bson.M{"user_id": user_token.UserId}).All(&user_posts)
			
			if err != nil {
				panic(err)
			}
			
			// Get the posts where the user commented
			err = database.C("posts").Find(bson.M{"users": user_token.UserId, "user_id": bson.M{"$ne":  user_token.UserId}}).All(&commented_posts)
			
			if err != nil {
				panic(err)
			}
			
			for _, post := range user_posts {
				
				activity = append(activity, model.UserActivity{
					Title: post.Title,
					Content: post.Content,
					Created: post.Created,
					Directive: "owner",
					Author: map[string]string{
						"id":    user.Id.Hex(),
						"name":  user.UserName,
						"email": user.Email,
					},
				})
			}
			
			for _, post := range commented_posts {
				
				for _, comment := range post.Comments.Set {
					
					if comment.UserId == user.Id {
						
						activity = append(activity, model.UserActivity{
							Title: post.Title,
							Content: comment.Content,
							Created: comment.Created,
							Directive: "commented",
							Author: map[string]string{
								"id":    user.Id.Hex(),
								"name":  user.UserName,
								"email": user.Email,
							},
						})
					}
				}
			}
			
			// Sort the full set of posts by the time they happened
			sort.Sort(model.ByCreatedAt(activity))
			
			c.JSON(200, gin.H{"activity": activity})
    	}
	}
}

func (di *UserAPI) UserAutocompleteGet(c *gin.Context) {
	
	// Get the database interface from the DI
	database := di.DataService.Database

	var users []gin.H
	
	qs := c.Request.URL.Query()
	name := qs.Get("search")
	
	if name != "" {
		
		err := database.C("users").Find(bson.M{"username": bson.RegEx{"^" + name, "i"}}).Select(bson.M{"_id": 1, "username": 1, "email": 1}).All(&users)
		
		if err != nil {
			panic(err)
		}
		
		c.JSON(200, gin.H{"users": users})
	}
}