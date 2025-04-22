test: test.o
	ld test.o -o test

test.o: test.s
	as test.s -o test.o

test.s: mgbfc test.bf
	./mgbfc test.bf

mgbfc: mgbfc.go
	go build mgbfc.go

# BUILD_DIR:=build

# $(BUILD_DIR)/test: $(BUILD_DIR)/test.o
# 	ld $(BUILD_DIR)/test.o -o $(BUILD_DIR)/test

# $(BUILD_DIR)/test.o: $(BUILD_DIR)/test.asm
# 	as $(BUILD_DIR)/test.asm -o $(BUILD_DIR)/test.o

# $(BUILD_DIR)/test.asm: mgbfc.go test.bf
# 	@mkdir -p $(@D)
# 	go run mgbfc.go
