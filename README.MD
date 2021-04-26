参考
https://qiita.com/kuboshu83/items/f827ad7068550cded72d
https://blog.potproject.net/2019/02/06/go-ssh-mysql

```testing
cp ~/.ssh/id_rsa.pub ./config_files/id_rsa.pub
docker-compose build
docker-compose up

ssh root@localhost -p 20022 ※ 接続確認
go run sensitive_collection_tool.go
```

※ 本番環境向けにprivateに移行されたので残骸で大分古い情報です
