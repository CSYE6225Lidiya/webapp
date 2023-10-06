# webapp

# Pre requisites for building and deploying the application locally

1. MySQL Installation

2. Go Installation

# Build and Deploy Instructions

1. Go to the webapp directory and run the below command to download dependencies.
    
    go mod download

2. Build the code using

    go build main.go

3. Set the env varibales for db connection

    export DB_USER=<value>
    export DB_PASSWORD=<value>
    export DB_NAME=<value>
    export DB_HOST=<value>
    export DB_PORT=<value>
    export USERS_PATH=<value>

4. Deploy the app by running the binary created in above step
   
   ./main.exe (windows machine)

   ./main (ubuntu machine)

5. Hit the various endpoints using the corresponding URLs
