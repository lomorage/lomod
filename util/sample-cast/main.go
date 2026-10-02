package main

import (
	"context"
	"log"

	agent "bitbucket.org/lomoware/lomo-backend/common/castagent"
	"github.com/sirupsen/logrus"
)

func main() {
	notify := make(chan agent.Notify)
	insert := make(chan agent.CastItem)
	logrus.SetLevel(logrus.DebugLevel)
	c := agent.New(context.Background(), "10.0.1.19", 8009, notify)

	go func() {
		pos := 0
		for status := range notify {
			if status.Status == agent.CastPlaying {
				pos++
			}
			if pos == 1 {
				insert <- agent.CastItem{
					PreloadTime: 5, PlaybackDuration: 20,
					ContentURL:  "http://10.0.1.13:8000/true_2018_07_28.png",
					ContentType: "image/png",
				}
			}
		}
	}()

	items := []agent.CastItem{}
	files := []string{"true_2003_01_17.jpg", "true_2003_11_01_2.jpg", "true_2003_11_01_2.jpg", "true_2003_11_23.jpg", "true_2004_1_21.jpg"}
	for _, f := range files {
		items = append(items, agent.CastItem{
			PreloadTime: 5, PlaybackDuration: 20,
			ContentURL:  "http://10.0.1.13:8000/" + f,
			ContentType: "image/jpeg",
		})
	}
	err := c.SlideShow(items, insert)
	if err != nil {
		log.Fatal(err)
	}
}
