FROM google/golang

RUN apt-get install ssh -y
WORKDIR /gopath/src/app
ADD . /gopath/src/app/
RUN go get app

CMD go install ; echo /gopath/bin/app
