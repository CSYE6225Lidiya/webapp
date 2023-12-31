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

# Build and Deploy Instructions on AWS

1. When a PR is merged, AMI will be generated.

2. Use pulumi to bring up the ec2 instance

3. Connect to ec2 instance using ssh and run the app 

4. Hit the URLs from any rest client andd test the APIs

# SSL Cerificate

SSl certificates are used form zerssl for demo. The command used to import certificate is given below:

    aws acm import-certificate --certificate fileb://certificate.crt --certificate-chain fileb://ca_bundle.crt --private-key fileb://private.key

*Set aws profile before executing the command
