package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v4"
)

// --- MODELLER ---
type Option struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type Question struct {
	ID      int      `json:"id"`
	Text    string   `json:"text"`
	Options []Option `json:"options"`
}

func main() {
	// 1. NEON DB BAĞLANTISI
	connStr := "postgresql://neondb_owner:npg_osBAh35rWzVC@ep-gentle-waterfall-alr3uyjz-pooler.c-3.eu-central-1.aws.neon.tech/neondb?sslmode=require"
	db, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Veritabanına bağlanılamadı: ", err)
	}
	defer db.Close(context.Background())
	fmt.Println("✅ Neon DB Bağlantısı Başarılı!")

	app := fiber.New()

	// 2. CORS AYARI
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// 3. FRONTEND DOSYALARI (out klasöründen kopyalayıp static'e attıkların)
	app.Static("/", "./static")

	// 4. GİRİŞ (LOGIN)
	app.Post("/login", func(c *fiber.Ctx) error {
		type LoginRequest struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Geçersiz istek"})
		}
		var dbPassword string
		err := db.QueryRow(context.Background(), "SELECT password FROM users WHERE email=$1", req.Email).Scan(&dbPassword)
		if err != nil || dbPassword != req.Password {
			return c.Status(401).JSON(fiber.Map{"error": "E-posta veya şifre hatalı!"})
		}
		return c.JSON(fiber.Map{"token": "gizli-anahtar-123", "success": true})
	})

	// 5. SORULARI VE ŞIKLARI GETİR
	app.Get("/questions", func(c *fiber.Ctx) error {
		query := `
			SELECT q.id, q.text, 
			       json_agg(json_build_object('id', o.id, 'text', o.option_text) ORDER BY o.id) as options
			FROM questions q
			LEFT JOIN options o ON q.id = o.question_id
			GROUP BY q.id, q.text
			ORDER BY q.id
		`
		rows, err := db.Query(context.Background(), query)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Veriler çekilemedi"})
		}
		defer rows.Close()

		var questions []Question
		for rows.Next() {
			var q Question
			var optionsJSON []byte
			if err := rows.Scan(&q.ID, &q.Text, &optionsJSON); err != nil {
				continue
			}
			json.Unmarshal(optionsJSON, &q.Options)
			questions = append(questions, q)
		}
		return c.JSON(questions)
	})

	// 6. ANKETİ BİTİR, CEVAPLARI KAYDET VE TOKEN VER
	app.Post("/finish-survey", func(c *fiber.Ctx) error {
		type AnswerReq struct {
			QuestionID int `json:"question_id"`
			OptionID   int `json:"option_id"`
		}
		type FinishRequest struct {
			Email   string      `json:"email"`
			Answers []AnswerReq `json:"answers"`
		}
		var req FinishRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Hata"})
		}

		// Kullanıcıyı bul
		var userID int
		err := db.QueryRow(context.Background(), "SELECT id FROM users WHERE email=$1", req.Email).Scan(&userID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Kullanıcı yok"})
		}

		// Cevapları döngüyle kaydet
		for _, ans := range req.Answers {
			_, _ = db.Exec(context.Background(),
				"INSERT INTO answers (user_id, question_id, option_id) VALUES ($1, $2, $3)",
				userID, ans.QuestionID, ans.OptionID)
		}

		// Token artır
		_, _ = db.Exec(context.Background(), "UPDATE users SET tokens = tokens + 10 WHERE id = $1", userID)

		return c.JSON(fiber.Map{"success": true, "message": "Anket kaydedildi, 10 token yüklendi!"})
	})

	// 7. SAYFA YENİLEME DESTEĞİ (SPA)
	app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile("./static/index.html")
	})

	// PORT AYARI
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 Sistem %s portunda aktif!\n", port)
	log.Fatal(app.Listen(":" + port))
}
