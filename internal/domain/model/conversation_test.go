package model

import (
	"errors"
	"testing"
	"time"
)

var product = ProductSnapshot{
	ID: "5e1f0c2a-7b3d-4e8f-9a6b-000000000021", Name: "Mora de Castilla",
	ProducerID: "0b6f2c7e-4a51-4c1e-9d0e-2f1c3a7b9e10", ProducerName: "Ana Ruiz",
}

const buyer = "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d"

func TestOpenCopiesTheSnapshots(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	c, err := Open("7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e", product, buyer, "Carlos Pérez", now)
	if err != nil {
		t.Fatal(err)
	}
	if c.ProductName != "Mora de Castilla" || c.ProducerID != product.ProducerID || c.ProducerName != "Ana Ruiz" ||
		c.BuyerID != buyer || c.BuyerName != "Carlos Pérez" || !c.CreatedAt.Equal(now) {
		t.Fatalf("snapshots not copied: %+v", c)
	}
	if c.AgreedPurchaseMethod != nil || c.LastMessageAt != nil {
		t.Fatal("no method and no message when it opens")
	}
}

func TestOpenRejectsAProducerTalkingToHimself(t *testing.T) {
	if _, err := Open("id", product, product.ProducerID, "Ana Ruiz", time.Now()); err == nil {
		t.Fatal("the buyer and the producer of a conversation are different people")
	}
}

func TestOpenNeedsTheNames(t *testing.T) {
	if _, err := Open("id", product, buyer, " ", time.Now()); err == nil {
		t.Fatal("the buyer name is a required snapshot (D-C31)")
	}
	noName := product
	noName.Name = ""
	if _, err := Open("id", noName, buyer, "Carlos", time.Now()); err == nil {
		t.Fatal("the product name is a required snapshot")
	}
}

func TestOnlyParticipants(t *testing.T) {
	c, _ := Open("id", product, buyer, "Carlos Pérez", time.Now())
	if c.EnsureParticipant(buyer) != nil || c.EnsureParticipant(product.ProducerID) != nil {
		t.Fatal("the buyer and the producer take part")
	}
	if err := c.EnsureParticipant("someone-else"); !errors.Is(err, ErrNotAParticipant) {
		t.Fatalf("got %v", err)
	}
}

func TestPurchaseMethodIsAClosedEnum(t *testing.T) {
	for _, ok := range []string{"PLATFORM", "OFF_PLATFORM"} {
		if _, err := ParsePurchaseMethod(ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "platform", "CASH"} {
		if _, err := ParsePurchaseMethod(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestChoosePurchaseMethodKeepsTheLastChoice(t *testing.T) {
	c := Conversation{ID: "c"}
	c.ChoosePurchaseMethod(OffPlatform)
	c.ChoosePurchaseMethod(Platform)
	if c.AgreedPurchaseMethod == nil || *c.AgreedPurchaseMethod != Platform {
		t.Fatalf("last write wins: %v", c.AgreedPurchaseMethod)
	}
}
