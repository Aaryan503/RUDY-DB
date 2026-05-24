<h1>RUDY DB</h1>
RUDY is a Mini Database Engine that I am trying to build from scratch. It is built in Golang. For now it supports creating tables, insertion of rows, deletion of rows and tables, and basic retrieval of rows and tables

<h2>Setup</h2>

Firstly, ensure that you download Go from the online sources
Next
Clone repository:
```bash
git clone https://github.com/Aaryan503/RUDY-DB
cd RUDY-DB
go mod tidy
go run .
```

For now, the server by default starts at localhost:8080

<h2> API Examples</h2>

Create a table:
```bash
curl -X POST localhost:8080/tables/footballers \
-H "Content-Type: application/json" \
-d '{
    "columns":[
        {"name":"name","type":"string"},
        {"name":"age","type":"int"},
        {"name":"active","type":"bool"},
        {"name": "goals_scored", "type": "int"},
        {"name": "jersey_number", "type": "int"}
    ]
}'
```

Insert row:
```bash
curl -X POST localhost:8080/tables/footballers/rows/1 \
-H "Content-Type: application/json" \
-d '{
    "name":"Lionel Messi",
    "age":38,
    "active":true,
    "goals_scored": 910,
    "jersey_number" : 10
}'
```

Get table:
```bash
curl localhost:8080/tables/footballer
```

Get all tables:
```bash
curl localhost:8080/tables
```

Get a particular row:
```bash
curl localhost:8080/tables/footballers/rows/1
```

Delete an entire table:
```bash
curl -X DELETE localhost:8080/tables/footballers
```

Delete row:
```bash
curl -X DELETE localhost:8080/tables/footballer/row/1
```
