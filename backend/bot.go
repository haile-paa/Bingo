// backend/bot.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	msgChoose  = "አማራጭ ይምረጡ፦"
	msgNeedReg = "መጀመሪያ «ተመዝገብ» ን ተጭነው ስልክ ቁጥርዎን ያጋሩ።"
	msgErr     = "ችግር ተፈጥሯል። እባክዎ እንደገና ይሞክሩ።"
	msgHelp    = "📖 መመሪያ\n\n1. «ተመዝገብ» ን ተጭነው ስልክ ቁጥርዎን ያጋሩ።\n2. «ገንዘብ አስገባ» ን ተጭነው ከ50 ብር ጀምሮ በቴሌብር ይላኩ፣ የደረሰዎትን SMS ለቦቱ paste ያድርጉ።\n3. «ተጫወት» ን ተጭነው ካርቴላ ይምረጡ (አንዱ 10 ብር፣ ከአንድ በላይ መያዝ ይችላሉ)።\n4. ካርቴላ ለመምረጥ 30 ሰከንድ አለ፤ ከዚያ ቁጥሮች ይጠራሉ። የተጠራ ቁጥር በካርቴላዎ ላይ አረንጓዴ ይሆናል።\n5. መስመር የሞላ የመጀመሪያው ካርቴላ ያሸንፋል። ሁለት ካሸነፉ ሽልማቱ እኩል ይካፈላል፤ ማንም ካላሸነፈ ክፍያ ይመለሳል።"
)

var httpc = &http.Client{Timeout: 10 * time.Second}

type tgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type Message struct {
	Text    string `json:"text"`
	From    tgUser `json:"from"`
	Contact *struct {
		Phone  string `json:"phone_number"`
		UserID int64  `json:"user_id"`
	} `json:"contact"`
}

type Update struct {
	Message  *Message `json:"message"`
	Callback *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From tgUser `json:"from"`
	} `json:"callback_query"`
}

func tg(method string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("tg marshal:", err)
		return
	}
	resp, err := httpc.Post("https://api.telegram.org/bot"+cfg.BotToken+"/"+method, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Println("tg:", err)
		return
	}
	resp.Body.Close()
}

func notify(uid int64, text string) { tg("sendMessage", gin.H{"chat_id": uid, "text": text}) }

func sendText(uid int64, text string, markup any) {
	p := gin.H{"chat_id": uid, "text": text}
	if markup != nil {
		p["reply_markup"] = markup
	}
	tg("sendMessage", p)
}

func menuKB() gin.H {
	return gin.H{"inline_keyboard": [][]gin.H{
		{{"text": "🎮 ተጫወት", "web_app": gin.H{"url": cfg.MiniAppURL}}, {"text": "📝 ተመዝገብ", "callback_data": "reg"}},
		{{"text": "💵 ሂሳብ ይመልከቱ", "callback_data": "bal"}, {"text": "💵 ገንዘብ አስገባ", "callback_data": "dep"}},
		{{"text": "☎️ ድጋፍ", "callback_data": "sup"}, {"text": "📖 መመሪያ", "callback_data": "ins"}},
		{{"text": "🤑 ገንዘብ አውጣ", "callback_data": "wd"}, {"text": "🔗 ጋብዝ", "callback_data": "inv"}},
	}}
}

func sendMenu(uid int64, text string) { sendText(uid, text, menuKB()) }

func loadUser(ctx context.Context, uid int64) (User, error) {
	var u User
	err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u)
	return u, err
}

func setState(ctx context.Context, uid int64, state string) {
	upd := bson.M{"$unset": bson.M{"state": ""}}
	if state != "" {
		upd = bson.M{"$set": bson.M{"state": state}}
	}
	if _, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid}, upd); err != nil {
		log.Println("state:", err)
	}
}

func botWebhook(c *gin.Context) {
	if c.Param("secret") != cfg.WebhookSecret {
		c.Status(404)
		return
	}
	var up Update
	if err := c.ShouldBindJSON(&up); err != nil {
		c.Status(400)
		return
	}
	c.Status(200)
	go handleUpdate(up)
}

func handleUpdate(up Update) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if up.Callback != nil {
		onCallback(ctx, up.Callback.ID, up.Callback.From, up.Callback.Data)
		return
	}
	if up.Message == nil || up.Message.From.ID == 0 {
		return
	}
	onMessage(ctx, up.Message)
}

func askContact(uid int64, u User) {
	if u.Phone != "" {
		sendMenu(uid, "ቀድሞ ተመዝግበዋል ✅")
		return
	}
	sendText(uid, "📱 ምዝገባዎን ለመጨረስ ከታች ያለውን «Share contact» ይጫኑ።", gin.H{
		"keyboard":          [][]gin.H{{{"text": "📞 Share contact", "request_contact": true}}},
		"resize_keyboard":   true,
		"one_time_keyboard": true,
	})
}

func onContact(ctx context.Context, m *Message) {
	uid := m.From.ID
	if m.Contact.UserID != uid { // only your own contact counts
		sendText(uid, "እባክዎ የራስዎን ስልክ ቁጥር ያጋሩ።", nil)
		return
	}
	if _, err := ensureUser(ctx, uid, m.From.FirstName, 0); err != nil {
		log.Println("contact:", err)
		return
	}
	switch registerPhone(ctx, uid, m.Contact.Phone) {
	case "ok":
		sendText(uid, "🎉 ✅ በተሳካ ሁኔታ ተመዝግበዋል!\n🎁 "+birr(signupBonus)+" ብር የጨዋታ ስጦታ ተጨምሯል።", gin.H{"remove_keyboard": true})
		sendMenu(uid, msgChoose)
	case "dup":
		sendText(uid, "ይህ ስልክ ቁጥር በሌላ አካውንት ተመዝግቧል።", gin.H{"remove_keyboard": true})
	case "already":
		sendText(uid, "ቀድሞ ተመዝግበዋል ✅", gin.H{"remove_keyboard": true})
		sendMenu(uid, msgChoose)
	case "bad":
		sendText(uid, "የኢትዮጵያ ስልክ ቁጥር (09… ወይም 07…) ብቻ ተቀባይነት አለው።", gin.H{"remove_keyboard": true})
	default:
		sendText(uid, msgErr, nil)
	}
}

func onStart(ctx context.Context, from tgUser, text string) {
	var ref int64
	if f := strings.Fields(text); len(f) > 1 && strings.HasPrefix(f[1], "ref_") {
		ref, _ = strconv.ParseInt(strings.TrimPrefix(f[1], "ref_"), 10, 64)
	}
	if _, err := ensureUser(ctx, from.ID, from.FirstName, ref); err != nil {
		log.Println("start:", err)
		return
	}
	u, err := loadUser(ctx, from.ID)
	if err != nil {
		log.Println("start load:", err)
		return
	}
	setState(ctx, from.ID, "")
	if u.Phone == "" {
		sendText(from.ID, "👋 እንኳን ደህና መጡ, "+from.FirstName+"! ለመጀመር ይመዝገቡ፤ ሲመዘገቡ "+birr(signupBonus)+" ብር ስጦታ ያገኛሉ።", nil)
		askContact(from.ID, u)
		return
	}
	sendMenu(from.ID, "👋 እንኳን ደህና መጡ, "+from.FirstName+"! "+msgChoose)
}

func parseBirr(text string, min int64, max int64) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || v*100 < min || v > max {
		return 0, false
	}
	return v, true
}

func onMessage(ctx context.Context, m *Message) {
	uid, text := m.From.ID, strings.TrimSpace(m.Text)
	if m.Contact != nil {
		onContact(ctx, m)
		return
	}
	if strings.HasPrefix(text, "/start") {
		onStart(ctx, m.From, text)
		return
	}
	u, err := loadUser(ctx, uid)
	if err != nil {
		sendMenu(uid, msgChoose)
		return
	}
	switch u.State {
	case "dep_amount":
		amt, ok := parseBirr(text, minDeposit, 100000)
		if !ok {
			sendText(uid, "ከ"+birr(minDeposit)+" ብር ጀምሮ ትክክለኛ ቁጥር ያስገቡ።", nil)
			return
		}
		setState(ctx, uid, "dep_sms")
		sendText(uid, "1. ከታች ባለው የቴሌብር አካውንት "+strconv.FormatInt(amt, 10)+" ብር ያስገቡ\n\nስም፦ "+cfg.TelebirrName+"\nስልክ፦ "+cfg.TelebirrNumber+"\n\n2. የከፈሉበትን አጭር የጽሁፍ መልዕክት (SMS) copy በማድረግ እዚህ ላይ paste አድርገው ይላኩት 👇", nil)
	case "dep_sms":
		reply, ok := createDeposit(ctx, uid, text)
		if !ok {
			sendText(uid, reply, nil)
			return
		}
		setState(ctx, uid, "")
		sendMenu(uid, reply)
	case "wd_amount":
		amt, ok := parseBirr(text, minWithdraw, 1000000)
		if !ok {
			sendText(uid, "ከ"+birr(minWithdraw)+" ብር ጀምሮ ትክክለኛ ቁጥር ያስገቡ።", nil)
			return
		}
		switch withdrawTo(ctx, uid, u.Phone, amt*100) {
		case "ok":
			setState(ctx, uid, "")
			sendMenu(uid, "✅ ጥያቄዎ ተልኳል። አድሚን በቴሌብር ይልካል።")
		case "funds":
			sendText(uid, "በቂ የሚወጣ ሂሳብ የለዎትም። የጨዋታ ዋሌት ለመጫወት ብቻ ነው።", nil)
		default:
			sendText(uid, msgErr, nil)
		}
	default:
		sendMenu(uid, msgChoose)
	}
}

func onCallback(ctx context.Context, id string, from tgUser, data string) {
	tg("answerCallbackQuery", gin.H{"callback_query_id": id})
	uid := from.ID
	if _, err := ensureUser(ctx, uid, from.FirstName, 0); err != nil {
		log.Println("callback:", err)
		return
	}
	u, err := loadUser(ctx, uid)
	if err != nil {
		log.Println("callback load:", err)
		return
	}
	setState(ctx, uid, "")
	if u.Phone == "" && data != "reg" && data != "sup" && data != "ins" {
		sendText(uid, msgNeedReg, nil)
		askContact(uid, u)
		return
	}
	switch data {
	case "reg":
		askContact(uid, u)
	case "bal":
		sendMenu(uid, "💼 የአካውንት መረጃ\n\nስም፦ "+u.Name+"\nስልክ፦ "+u.Phone+"\nዋና ዋሌት፦ "+birr(u.Balance)+" ብር\nየጨዋታ ዋሌት፦ "+birr(u.Bonus)+" ብር")
	case "dep":
		setState(ctx, uid, "dep_amount")
		sendText(uid, "💰 ማስገባት የሚፈልጉትን መጠን ከ"+birr(minDeposit)+" ብር ጀምሮ ያስገቡ።", nil)
	case "wd":
		setState(ctx, uid, "wd_amount")
		sendText(uid, "🤑 ማውጣት የሚፈልጉትን መጠን (ብር) ያስገቡ። ዝቅተኛው "+birr(minWithdraw)+" ብር ነው።\nየሚወጣ ሂሳብ፦ "+birr(u.Balance)+" ብር\nገንዘቡ ወደ "+u.Phone+" በቴሌብር ይላካል።", nil)
	case "inv":
		link := "https://t.me/" + cfg.BotUsername + "?start=ref_" + strconv.FormatInt(uid, 10)
		sendMenu(uid, "🎁 ጓደኞችዎን ይጋብዙ፦\n"+link+"\n\nጓደኛዎ ሲመዘገብ "+birr(signupBonus)+" ብር ያገኛል። የመጀመሪያ ገቢው ሲጸድቅ እርስዎ "+birr(referralBonus)+" ብር ያገኛሉ።")
	case "sup":
		if cfg.SupportUser == "" {
			sendMenu(uid, "ድጋፍ በቅርቡ ይጨመራል።")
			return
		}
		sendMenu(uid, "☎️ ድጋፍ ለማግኘት፦ @"+cfg.SupportUser)
	case "ins":
		sendMenu(uid, msgHelp)
	}
}
