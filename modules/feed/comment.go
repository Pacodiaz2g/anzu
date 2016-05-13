package feed

import (
	"gopkg.in/mgo.v2/bson"

	"html"
	"time"
)

type Comment struct {
	Id       bson.ObjectId `bson:"_id,omitempty" json:"id,omitempty"`
	PostId   bson.ObjectId `bson:"post_id" json:"post_id"`
	UserId   bson.ObjectId `bson:"user_id" json:"user_id"`
	Votes    Votes         `bson:"votes" json:"votes"`
	User     interface{}   `bson:"-" json:"author,omitempty"`
	Position int           `bson:"position" json:"position"`
	Liked    int           `bson:"-" json:"liked,omitempty"`
	Content  string        `bson:"content" json:"content"`
	Chosen   bool          `bson:"chosen,omitempty" json:"chosen,omitempty"`
	Created  time.Time     `bson:"created_at" json:"created_at"`

	// Runtime generated pointers
	post *Post
}

type Comments struct {
	Count  int        `bson:"count" json:"count"`
	Total  int        `bson:"-" json:"total"`
	Answer *Comment   `bson:"-" json:"answer,omitempty"`
	Set    []*Comment `bson:"-" json:"set"`
}

type CSortByCreatedAt []*Comment

func (a CSortByCreatedAt) Len() int           { return len(a) }
func (a CSortByCreatedAt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a CSortByCreatedAt) Less(i, j int) bool { return !a[i].Created.Before(a[j].Created) }

func (self *Comment) SetDI(o *Post) {
	self.post = o
}

func (self *Comment) GetContent() string {
	return self.Content
}

func (self *Comment) UpdateContent(c string) bool {
	self.Content = c
	return true
}

func (self *Comment) OnParseFilterFinished(module string) bool {
	return true
}

func (self *Comment) OnParseFinished() bool {
	return true
}

func (self *Comment) GetParseableMeta() map[string]interface{} {

	return map[string]interface{}{
		"id":      self.Id,
		"type":    "comment",
		"comment": self,
	}
}

func (self *Comment) GetPost() *Post {
	return self.post
}

func (self *Comment) MarkAsAnswer() {

	// Get database instance
	database := self.post.DI().Mongo.Database

	// Update straight forward
	err := database.C("comments").Update(bson.M{"_id": self.Id}, bson.M{"$set": bson.M{"chosen": true}})

	if err != nil {
		panic(err)
	}

	err = database.C("posts").Update(bson.M{"_id": self.PostId}, bson.M{"$set": bson.M{"solved": true}})

	if err != nil {
		panic(err)
	}
}

func (self *Comment) Delete() {

	// Get database instance
	database := self.post.DI().Mongo.Database

	// Update straight forward
	err := database.C("comments").Update(bson.M{"_id": self.Id}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})

	if err != nil {
		panic(err)
	}

	// Decrement count of comments
	err = database.C("posts").Update(bson.M{"_id": self.PostId}, bson.M{"$inc": bson.M{"comments.set.count": -1}})

	if err != nil {
		panic(err)
	}
}

func (self *Comment) Update(c string) {

	if len(c) > 0 {

		self.Content = html.EscapeString(c)

		// Use content module to run processors chain
		database := self.post.DI().Mongo.Database
		content := self.post.DI().Content
		content.Parse(self)

		// Update database with new content
		err := database.C("comments").Update(bson.M{"_id": self.Id}, bson.M{"$set": bson.M{"content": self.Content, "updated_at": time.Now()}})

		if err != nil {
			panic(err)
		}

		// Finally parse the tags
		content.ParseTags(self)
	}
}
