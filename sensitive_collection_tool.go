package main

import (
	"database/sql"
	// "encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"bytes"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/go-sql-driver/mysql" //mysql driver
	"golang.org/x/crypto/ssh"
)

type (
	// returnCode -- 処理結果
	returnCode int
)

const (
	success                   returnCode = iota // 成功
	homedirNotFound                             // $HOME が展開出来なかった
	readErrSSHPrivateKey                        // 秘密鍵読み取り中にエラー
	parseErrSSHPrivateKey                       // 秘密鍵の解析中にエラー
	connErrSSHClient                            // SSH接続中にエラー
	canNotCreateNewSSHSession                   // SSHにてセッションを生成中にエラー
	execErrInSSHSession                         // SSHにてコマンドを実行中にエラー
)

func main() {
	now := time.Now()
	settings := load_json()
	settings.ForEach(func(key, value gjson.Result) bool {
		exec_cmd(now.Format("2006-01-02"), []string{"cat", "/etc/passwd"}, "passwd", key.String(), value)
		exec_cmd(now.Format("2006-01-02"), []string{"cat", "/etc/group"}, "group", key.String(), value)
		exec_mysql(now.Format("2006-01-02"), "select Host, User from mysql.user;", "mysql", key.String(), value)
		create_mail_template(now, key.String())
		// send_mail(now, key.String(), value)
		return true
	})
}

func readFile(filepath string) string {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		fmt.Println(os.Stderr, err)
		os.Exit(1)
	}
	data := string(b)

	return data
}

func timeFormat(now time.Time) string {
	weekday := [...]string{"月曜日", "火曜日", "水曜日", "木曜日", "金曜日", "土曜日", "日曜日"}
	location := time.FixedZone("JST", 9*60*60)
	now = now.In(location)
	zone, _ := now.Zone()
	return fmt.Sprintf("%d年 %d月 %d日 %s %d:%d:%d %s\n", now.Year(), now.Month(), now.Day(), weekday[now.Weekday()], now.Hour(), now.Minute(), now.Second(), zone)
}

func create_mail_template(now time.Time, key string) {
	merge := timeFormat(now)
	merge = merge + mergeFiles("passwd", "cat /etc/passwd", key)
	merge = merge + mergeFiles("group", "cat /etc/group", key)
	merge = merge + mergeFiles("mysql", "select Host, User from mysql.user;", key)
	slice := strings.Split(merge, "\n")
	lines := []string{}
	for _, str := range slice {
		if !strings.HasPrefix(str, "#") {
			lines = append(lines, str+"\n")
		}
	}

	b := []byte{}
	for _, line := range lines {
		if line == "\n" {
			continue
		}
		ll := []byte(line)
		for _, l := range ll {
			b = append(b, l)
		}
	}

	err := ioutil.WriteFile("output_mail.txt", b, 0666)
	if err != nil {
		fmt.Println(os.Stderr, err)
		os.Exit(1)
	}
}

func mergeFiles(cmd_type string, cmd string, key string) string {
	// base読み込み
	base := readFile("./template/base_template.txt")
	filepaths := dirwalk("./tmp")
	merge_content := ""
	for _, path := range filepaths {
		if (strings.Index(path, cmd_type) != -1) && (strings.Index(path, key) != -1) {
			content := readFile("./template/content_template.txt")
			target := readFile(path)

			filename := filepath.Base(path)
			split_name := strings.Split(filename, "_")
			content = strings.Replace(content, "server", split_name[0], -1)
			content = strings.Replace(content, "user", target, -1)
			merge_content = merge_content + content
		}
	}
	base = strings.Replace(base, "command", cmd, -1)
	base = strings.Replace(base, "content", merge_content, -1)
	return base
}

// 再起ファイル取得
func dirwalk(dir string) []string {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var paths []string
	for _, file := range files {
		if file.IsDir() {
			paths = append(paths, dirwalk(filepath.Join(dir, file.Name()))...)
			continue
		}
		paths = append(paths, filepath.Join(dir, file.Name()))
	}

	return paths
}

// newDB はmysqlクライアントを生成する
func newDB(sshc *ssh.Client, settings gjson.Result) (*sql.DB, error) {
	mysqlNet := "tcp"
	if sshc != nil {
		mysqlNet = "mysql+tcp"
		dialFunc := func(addr string) (net.Conn, error) {
			return sshc.Dial("tcp", addr)
		}
		mysql.RegisterDial(mysqlNet, dialFunc)
	}
	dbConf := &mysql.Config{
		User:                 settings.Get("DATABASE_USER").String(),
		Passwd:               settings.Get("DATABASE_PASSWORD").String(),
		Addr:                 settings.Get("DATABASE_HOST").String() + ":" + settings.Get("DATABASE_PORT").String(),
		Net:                  mysqlNet,
		DBName:               "mysql",
		ParseTime:            true,
		AllowNativePasswords: true,
	}
	return sql.Open("mysql", dbConf.FormatDSN())
}

func exec_mysql(now string, cmd string, cmd_type string, key string, value gjson.Result) {
	sshClient, err := newSSH(value)
	if err != nil {
		fmt.Println("err in new ssh client. reason : " + err.Error())
	} else {
		defer sshClient.Close()
	}
	db, err := newDB(sshClient, value)
	if err != nil {
		log.Printf("Mysql server seems down: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query("select Host, User from mysql.user;")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	b := []byte{}
	ll := []byte("Host User\n")
	for _, l := range ll {
		b = append(b, l)
	}

	for rows.Next() {
		var user string
		var host string
		if err := rows.Scan(&host, &user); err != nil {
			log.Fatal(err)
		}

		ll := []byte(host + " " + user + "\n")
		for _, l := range ll {
			b = append(b, l)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	out, returnCode := sshWithKeyFileWithInsecureHostKey([]string{"hostname"}, value)
	if returnCode != success {
		fmt.Println(returnCode)
		os.Exit(1)
	}

	err = ioutil.WriteFile(fmt.Sprintf("./tmp/%s_%s_%s.txt", strings.TrimSpace(string(out)), key, cmd_type), b, 0666)
	if err != nil {
		fmt.Println(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(cmd_type + " complete!")
}

func exec_cmd(now string, cmd []string, cmd_type string, key string, value gjson.Result) {
	out, returnCode := sshWithKeyFileWithInsecureHostKey(cmd, value)
	if returnCode != success {
		fmt.Println(returnCode)
		os.Exit(1)
	}

	slice := strings.Split(string(out), "\n")
	lines := []string{}
	for _, str := range slice {
		if !strings.HasPrefix(str, "#") {
			lines = append(lines, str+"\n")
		}
	}

	b := []byte{}
	for _, line := range lines {
		if line == "\n" {
			continue
		}
		ll := []byte(line)
		for _, l := range ll {
			b = append(b, l)
		}
	}

	out, returnCode = sshWithKeyFileWithInsecureHostKey([]string{"hostname"}, value)
	if returnCode != success {
		fmt.Println(returnCode)
		os.Exit(1)
	}

	err := ioutil.WriteFile(fmt.Sprintf("./tmp/%s_%s_%s.txt", strings.TrimSpace(string(out)), key, cmd_type), b, 0666)
	if err != nil {
		fmt.Println(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(cmd_type + " complete!")
}

func load_json() gjson.Result {
	raw, err := ioutil.ReadFile("./settings.json")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	return gjson.Parse(string(raw))
}

func send_mail(now time.Time, key string, value gjson.Result) {
	setting := value
	creds := credentials.NewStaticCredentials(setting.Get("ACCESS_KEY_ID").String(), setting.Get("SECRET_ACCESS_KEY").String(), "")
	sess, err := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(setting.Get("REGION").String()),
	})
	if err != nil {
		fmt.Println(err)
	}
	svc := sns.New(sess)
	data := readFile("output_mail.txt")

	input := &sns.PublishInput{
		Message:  aws.String(data),
		TopicArn: aws.String(setting.Get("TOPIC_ARN").String()),
		Subject:  aws.String(fmt.Sprintf("【%s】OS/DBユーザー一覧_%s", key, now.Format("2006-01-02"))),
	}

	result, err := svc.Publish(input)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println(*result.MessageId)
}

// newSSH はsshクライアントを生成する
func newSSH(settings gjson.Result) (*ssh.Client, error) {
	// -------------------------------------------
	// $HOME/.ssh/id_rsa からデータ読み取り
	//
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	sshPrivKeyFile := filepath.Join(homeDir, ".ssh/id_rsa")
	privKey, err := ioutil.ReadFile(sshPrivKeyFile)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// -------------------------------------------
	// 秘密鍵を渡して Signer を取得
	//
	signer, err := ssh.ParsePrivateKey(privKey)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	hostKeyCallbackFunc := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}

	// -------------------------------------------
	// SSH の 接続設定 を構築
	//
	sshConf := &ssh.ClientConfig{
		// SSH ユーザ名
		User: settings.Get("SSH_USER").String(),
		// 認証方式
		Auth: []ssh.AuthMethod{
			// 鍵認証
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallbackFunc,
	}

	// -------------------------------------------
	// SSH で 接続
	//
	return ssh.Dial("tcp", settings.Get("SSH_HOST").String(), sshConf)
}

// ssh
func sshWithKeyFileWithInsecureHostKey(cmd []string, value gjson.Result) (string, returnCode) {
	// -------------------------------------------
	// SSH で 接続
	//
	conn, err := newSSH(value)
	if err != nil {
		log.Println(err)
		return "", connErrSSHClient
	}
	// -------------------------------------------
	// セッションを開いて、コマンドを実行
	//
	sess, err := conn.NewSession()
	if err != nil {
		log.Println(err)
		return "", canNotCreateNewSSHSession
	}
	defer sess.Close()

	// リモートサーバでのコマンド実行結果をローカルの標準出力と標準エラーへ流す
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err = sess.Run(strings.Join(cmd, " ")); err != nil {
		log.Println(err)
		return "", execErrInSSHSession
	}
	results := stdout.String()

	return results, success
}
