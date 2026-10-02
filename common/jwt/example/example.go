package main

import (
	"fmt"
	"strings"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/jwt"
)

func main() {
	secret := "ThisIsMySuperSecret"
	algorithm := jwt.HmacSha256(secret)

	claims := jwt.NewClaim()
	claims.Set("Role", "Admin")
	claims.SetTime("exp", time.Now().Add(time.Minute))

	token, err := algorithm.Encode(claims)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Token: %s\n", token)

	if algorithm.Validate(token) != nil {
		panic(err)
	}

	loadedClaims, err := algorithm.Decode(token)
	if err != nil {
		panic(err)
	}

	role, err := loadedClaims.Get("Role")
	if err != nil {
		panic(err)
	}

	roleString, ok := role.(string)
	if !ok {
		panic(err)
	}

	if strings.Compare(roleString, "Admin") == 0 {
		//user is an admin
		fmt.Println("User is an admin")
	}

	// decode with structure directory
	var newRole struct {
		Role string
	}

	err = algorithm.DecodeWithBody(token, &newRole)
	if err != nil {
		panic(err)
	}

	fmt.Printf("decode with object directly: %s\n", newRole.Role)
}
