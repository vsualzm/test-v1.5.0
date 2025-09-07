- Service inventory
go run ./inventorysvc

- Service Order
go run ./ordersvc


# stok awal SKU-ABC = 10
curl -X POST http://localhost:8080/orders/create \
  -H "Content-Type: application/json" \
  -d '{"sku":"SKU-ABC","qty":3}'

# contoh respons:
# {
#   "status":"OK",
#   "message":"Order created",
#   "remaining":7
# }

# sku tanpa stok
curl -X POST http://localhost:8080/orders/create \
  -H "Content-Type: application/json" \
  -d '{"sku":"SKU-XYZ","qty":1}'
# -> "Insufficient stock"
