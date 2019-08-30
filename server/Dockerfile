FROM ruby:2.6.3

RUN apt-get update -qq && apt-get install -y openssh-server && rm -rf /var/lib/apt/lists/*

ADD start-sshd.sh /bin/start-sshd.sh

EXPOSE 22
CMD ["/bin/start-sshd.sh"]
