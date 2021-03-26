FROM ubuntu:18.04

# openss-serverをインストール
RUN apt update && apt install -y openssh-server
RUN apt -y install libmysqld-dev libmysqlclient-dev mysql-client
# ssh用のディレクトリ作成
RUN mkdir /var/run/sshd
COPY ./config_files/id_rsa.pub /root/authorized_keys

RUN mkdir ~/.ssh && \
    mv ~/authorized_keys ~/.ssh/authorized_keys && \
    chmod 0600 ~/.ssh/authorized_keys

# rootでのSSHログインを許可
RUN sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config

# SSH login fix. Otherwise user is kicked off after login
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

EXPOSE 22
# sshdの起動
CMD ["/usr/sbin/sshd", "-D"]