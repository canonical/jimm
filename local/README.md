# Local Development

# Authentication Steps
1. docker compose up
2. Login candid with static user `CANDID_URL=http://localhost:8081 ./candidctl show -u jimm`
3. Update DB with:
    - -- INSERT INTO root_keys(id, created_at, expires, root_key) SELECT id, created, expires, rootkey FROM rootkeys;
    - -- INSERT INTO rootkeys(id, created, expires, rootkey) SELECT id, created_at, expires, root_key FROM root_keys ON CONFLICT DO NOTHING;
4. Get [second] macaroon from .go-cookies with:
    - cat ~/.go-cookies | jq -r '.[1].Value' | base64 --decode 
5. Login via JIMM Facade Admin, version 3, under /api (ws://localhost:17070/api) params:
    - ```json
    {
        "request-id": 5,
        "type": "Admin",
        "version": 3,
        "request": "Login",
        "params": {
            "auth-tag": "jimm",
            "credentials": "jimm",
            "macaroons": [[{"c":[{"i64":"AwA","v64":"BTapXjPQtsiUM1UfTZwhcorH_MjaYhlSJHnThcMJ4rwCJoU0NaQgYWyzb-0QU7BEl5Cfwm6OvSwrP4bT0EOxnWV_QIACZ5SD","l":"http://0.0.0.0:8081"},{"i":"time-before 2022-12-15T13:59:09.565428372Z"}],"l":"identity","i64":"AwoQOk5Pu6qg6nzo4wGDBlI97xIgNDMwOTljZDU0MWNmZTBhMjI4OGRiNTRiNWNiMGQ5NzcaDgoFbG9naW4SBWxvZ2lu","s64":"jktKBIF4cJKN9q97l6gPdmiRsClW2jfWahgCLA2fAiw"},{"c":[{"i":"declared username jimm"},{"i":"time-before 2022-12-15T13:59:15.206848339Z"}],"i64":"AwA","s64":"PnHNg0hF-OSjIX9j5x7k6cM9vx9Uj44DJGYjH04nKVM"}]]
        }
    }