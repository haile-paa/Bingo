// backend/finedit.go - the owner edits a player's wallets, name and phone (/finance -> People -> player).
package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const maxEditAmount int64 = 1_000_000_000 // 10,000,000 birr in santim

// finEdit: POST /finance/api/people/:id/edit   body {"balance": santim, "bonus": santim, "name": "...", "phone": "09..."}
// balance = prize wallet (withdrawable), bonus = play wallet. All fields optional; wallets are set to the given value.
func finEdit(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var in struct {
		Balance *int64  `json:"balance"`
		Bonus   *int64  `json:"bonus"`
		Name    *string `json:"name"`
		Phone   *string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
		return
	}
	for _, v := range []*int64{in.Balance, in.Bonus} {
		if v != nil && (*v < 0 || *v > maxEditAmount) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount out of range"})
			return
		}
	}
	users := db.Collection("users")
	var u User
	if err := users.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "player not found"})
		return
	}
	set := bson.M{}
	inc := bson.M{}
	var dBal, dBonus int64
	if in.Balance != nil {
		dBal = *in.Balance - u.Balance
		inc["balance"] = dBal // $inc by the difference, so a bet or prize that lands at the same moment is not lost
	}
	if in.Bonus != nil {
		dBonus = *in.Bonus - (u.Bonus + u.Promo)
		inc["bonus"] = dBonus + u.Promo
		set["promo"] = int64(0)
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > 60 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 1-60 characters"})
			return
		}
		set["name"] = name
	}
	if in.Phone != nil {
		phone := normPhone(*in.Phone)
		if phone == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad phone (09... or 07...)"})
			return
		}
		set["phone"] = phone
	}
	upd := bson.M{}
	if len(set) > 0 {
		upd["$set"] = set
	}
	if len(inc) > 0 {
		upd["$inc"] = inc
	}
	if len(upd) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to change"})
		return
	}
	if _, err := users.UpdateOne(ctx, bson.M{"_id": id}, upd); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "that phone belongs to another player"})
			return
		}
		log.Println("edit player:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server error"})
		return
	}
	if dBal != 0 {
		addTx(ctx, id, "admin_adjust", dBal, "prize")
	}
	if dBonus != 0 {
		addTx(ctx, id, "admin_adjust", dBonus, "play")
	}
	if _, err := db.Collection("adminlog").InsertOne(ctx, bson.M{"at": time.Now(), "action": "edit", "uid": id, "set": set, "dBalance": dBal, "dBonus": dBonus}); err != nil {
		log.Println("adminlog:", err)
	}
	log.Printf("PLAYER EDIT uid=%d dBalance=%d dBonus=%d", id, dBal, dBonus)
	peopleMu.Lock() // the People list must show the new numbers at once
	peopleCache = nil
	peopleMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
