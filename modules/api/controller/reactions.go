package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/tryanzu/core/board/comments"
	"github.com/tryanzu/core/board/posts"
	"github.com/tryanzu/core/board/votes"
	"github.com/tryanzu/core/core/events"
	"github.com/tryanzu/core/core/user"
	"github.com/tryanzu/core/deps"
	"gopkg.in/mgo.v2/bson"

	"net/http"
)

type upsertReactionBody struct {
	Type string `json:"type" binding:"required"`
}

// UpsertReaction realted to a reactable.
func UpsertReaction(c *gin.Context) {
	var (
		id      bson.ObjectId
		body    upsertReactionBody
		votable votes.Votable
		err     error
	)

	usr := c.MustGet("user").(user.User)
	if usr.Gaming.Swords < 15 {
		jsonErr(c, http.StatusPreconditionFailed, "Not enough user reputation.")
		return
	}

	// ID validation.
	if id = bson.ObjectIdHex(c.Params.ByName("id")); !id.Valid() {
		jsonErr(c, http.StatusBadRequest, "malformed request, invalid id")
		return
	}

	// Bind body data.
	if err = c.Bind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "reason": "Invalid request."})
		return
	}

	switch c.Params.ByName("type") {
	case "post":
		if post, err := post.FindId(deps.Container, id); err == nil {
			votable = post
		}
	case "comment":
		if comment, err := comments.FindId(deps.Container, id); err == nil {
			votable = comment
		}
	default:
		jsonErr(c, http.StatusBadRequest, "invalid type")
		return
	}

	if votable == nil {
		jsonErr(c, http.StatusNotFound, "invalid id")
		return
	}

	vote, status, err := votes.UpsertVote(deps.Container, votable, usr.Id, body.Type)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	// Events pool signal
	events.In <- events.Vote(vote)

	if vote.Deleted != nil {
		c.JSON(http.StatusOK, status)
		return
	}

	c.JSON(http.StatusOK, status)
}
