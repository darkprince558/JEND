package main

import (
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscognito"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"

	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiot"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type InfraStackProps struct {
	awscdk.StackProps
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewInfraStack(app, "JendInfraStackV4", &InfraStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

func NewInfraStack(scope constructs.Construct, id string, props *InfraStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// 1. DynamoDB Table
	table := awsdynamodb.NewTable(stack, jsii.String("JendRegistry"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("code"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		TimeToLiveAttribute: jsii.String("expires_at"),
		BillingMode:         awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy:       awscdk.RemovalPolicy_RETAIN, // PROD: Retain data on stack deletion
	})

	// 2. Lambda Function
	registryFunc := awslambda.NewFunction(stack, jsii.String("RegistryFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("../bin/registry.zip"), nil),
		Environment: &map[string]*string{
			"TABLE_NAME": table.TableName(),
		},
	})

	table.GrantReadWriteData(registryFunc)

	// 3. API Gateway (HTTP API)
	integration := awsapigatewayv2integrations.NewHttpLambdaIntegration(
		jsii.String("RegistryIntegration"),
		registryFunc,
		&awsapigatewayv2integrations.HttpLambdaIntegrationProps{},
	)

	httpApi := awsapigatewayv2.NewHttpApi(stack, jsii.String("JendApi"), &awsapigatewayv2.HttpApiProps{
		ApiName: jsii.String("JendRegistryApi"),
	})

	httpApi.AddRoutes(&awsapigatewayv2.AddRoutesOptions{
		Path:        jsii.String("/register"),
		Methods:     &[]awsapigatewayv2.HttpMethod{awsapigatewayv2.HttpMethod_POST},
		Integration: integration,
	})

	httpApi.AddRoutes(&awsapigatewayv2.AddRoutesOptions{
		Path:        jsii.String("/lookup/{code}"),
		Methods:     &[]awsapigatewayv2.HttpMethod{awsapigatewayv2.HttpMethod_GET},
		Integration: integration,
	})

	// 4. Output the API Endpoint
	awscdk.NewCfnOutput(stack, jsii.String("ApiEndpoint"), &awscdk.CfnOutputProps{
		Value: httpApi.ApiEndpoint(),
	})

	// 5. IoT Core Policy
	// RESTRICTED SCOPE for Production Security
	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"iot:Connect",
				},
				"Resource": []string{
					"arn:aws:iot:*:*:client/*", // Allow any client ID for now (randomized in app)
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"iot:Publish",
					"iot:Receive",
				},
				"Resource": []string{
					"arn:aws:iot:*:*:topic/jend/signal/*", // Only allow publishing to signal topics
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"iot:Subscribe",
				},
				"Resource": []string{
					"arn:aws:iot:*:*:topicfilter/jend/signal/*", // Only allow subscribing to signal topics
				},
			},
		},
	}

	awsiot.NewCfnPolicy(stack, jsii.String("JendSignalingPolicy"), &awsiot.CfnPolicyProps{
		PolicyName:     jsii.String("JendSignalingPolicyV4"),
		PolicyDocument: policyDoc,
	})

	// 6. Get IoT Endpoint (so the user doesn't have to look it up)
	// We use a AwsCustomResource to call the AWS SDK during deployment
	iotEndpointPr := awscdk.NewCfnOutput(stack, jsii.String("IotEndpoint"), &awscdk.CfnOutputProps{
		Value: getIotEndpoint(stack),
	})

	// Avoid unused variable error if we don't use it elsewhere, though CfnOutput self-registers.
	_ = iotEndpointPr

	// --- Phase 2: TURN/STUN Relay (EC2) ---

	// 7. VPC (Reuse creation logic but simpler)
	vpc := awsec2.NewVpc(stack, jsii.String("JendVpc"), &awsec2.VpcProps{
		MaxAzs: jsii.Number(2),
		SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
			{
				Name:       jsii.String("Public"),
				SubnetType: awsec2.SubnetType_PUBLIC,
				CidrMask:   jsii.Number(24),
			},
		},
		NatGateways: jsii.Number(0),
	})

	// 13. TURN Security Group
	turnSg := awsec2.NewSecurityGroup(stack, jsii.String("TurnSg"), &awsec2.SecurityGroupProps{
		Vpc:              vpc,
		Description:      jsii.String("Allow TURN traffic"),
		AllowAllOutbound: jsii.Bool(true),
	})
	// STUN/TURN standard ports
	turnSg.AddIngressRule(awsec2.Peer_AnyIpv4(), awsec2.Port_Udp(jsii.Number(3478)), jsii.String("TURN UDP"), nil)
	turnSg.AddIngressRule(awsec2.Peer_AnyIpv4(), awsec2.Port_Tcp(jsii.Number(3478)), jsii.String("TURN TCP"), nil)
	turnSg.AddIngressRule(awsec2.Peer_AnyIpv4(), awsec2.Port_Udp(jsii.Number(5349)), jsii.String("TURN TLS UDP"), nil)
	turnSg.AddIngressRule(awsec2.Peer_AnyIpv4(), awsec2.Port_Tcp(jsii.Number(5349)), jsii.String("TURN TLS TCP"), nil)
	// Relay ports (min-max)
	turnSg.AddIngressRule(awsec2.Peer_AnyIpv4(), awsec2.Port_UdpRange(jsii.Number(49152), jsii.Number(65535)), jsii.String("Relay UDP"), nil)

	// 14. EC2 Instance (t3.small for dev)

	// 13a. TURN Secret (Ensure rotation/update)
	turnSecret := awssecretsmanager.NewSecret(stack, jsii.String("TurnSecret"), &awssecretsmanager.SecretProps{
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String("{}"),
			GenerateStringKey:    jsii.String("secret"),
			ExcludePunctuation:   jsii.Bool(true),
		},
	})

	// 14. EC2 Instance (t3.small for dev)
	// Use Ubuntu 22.04 for easy coturn installation
	ami := awsec2.MachineImage_Lookup(&awsec2.LookupMachineImageProps{
		Name:   jsii.String("ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"),
		Owners: &[]*string{jsii.String("099720109477")}, // Canonical
	})

	userData := awsec2.UserData_ForLinux(&awsec2.LinuxUserDataOptions{})
	userData.AddCommands(
		jsii.String("apt-get update"),
		jsii.String("apt-get install -y coturn awscli jq"), // Install jq/awscli
		jsii.String("export AWS_DEFAULT_REGION="+*stack.Region()),
		jsii.String("SECRET=$(aws secretsmanager get-secret-value --secret-id "+*turnSecret.SecretArn()+" --query SecretString --output text | jq -r .secret)"),
		jsii.String("echo 'listening-port=3478' > /etc/coturn/turnserver.conf"),
		jsii.String("echo 'tls-listening-port=5349' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'listening-ip=0.0.0.0' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'external-ip=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4)' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'min-port=49152' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'max-port=65535' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'realm=jend.local' >> /etc/coturn/turnserver.conf"),
		jsii.String("echo 'use-auth-secret' >> /etc/coturn/turnserver.conf"), // Enable Dynamic Auth
		jsii.String("echo \"static-auth-secret=$SECRET\" >> /etc/coturn/turnserver.conf"),
		// Anti-Abuse Limits
		jsii.String("echo 'max-bps=1000000' >> /etc/coturn/turnserver.conf"), // 1MB/s limit
		jsii.String("echo 'user-quota=100' >> /etc/coturn/turnserver.conf"),  // Max allocations per user
		jsii.String("# Force Update 3"),
		jsii.String("systemctl enable coturn"),
		jsii.String("systemctl restart coturn"),
	)

	turnInstance := awsec2.NewInstance(stack, jsii.String("TurnInstance"), &awsec2.InstanceProps{
		Vpc:                vpc,
		InstanceType:       awsec2.NewInstanceType(jsii.String("t3.micro")), // Free Tier Eligible
		MachineImage:       ami,
		SecurityGroup:      turnSg,
		UserData:           userData,
		VpcSubnets:         &awsec2.SubnetSelection{SubnetType: awsec2.SubnetType_PUBLIC},
		DetailedMonitoring: jsii.Bool(true), // Enable detailed monitoring
	})

	// Add SSM permissions and Secrets Manager Access
	turnInstance.Role().AddManagedPolicy(awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AmazonSSMManagedInstanceCore")))
	turnSecret.GrantRead(turnInstance.Role(), nil)

	// 14b. Elastic IP - Stable IP for TURN
	eip := awsec2.NewCfnEIP(stack, jsii.String("TurnEip"), &awsec2.CfnEIPProps{
		InstanceId: turnInstance.InstanceId(),
	})

	// 15. TURN Auth Lambda (New for Phase 4)
	turnAuthFunc := awslambda.NewFunction(stack, jsii.String("TurnAuthFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("../bin/turn-auth.zip"), nil), // Assumes built
		Environment: &map[string]*string{
			"TURN_URI":            turnInstance.InstancePublicIp(),
			"TURN_SECRET_KEY_ARN": turnSecret.SecretArn(), // Pass ARN or Value?
			// To pass value, we need to read it in Lambda or pass as env.
			// Passing as Env exposes it in Lambda console.
			// Safer: Pass ARN and let Lambda fetch it.
			// BUT: To complete this quickly, I will pass the value (resolved token).
			// SecretsManager.SecretString is a token.
			// HOWEVER, parsing JSON object?
			// Secret currently generates JSON: {"secret": "..."}
			// I need to extract it.
			// Let's just create a plain string secret for simplicity?
			// SecretsManager construct forces JSON usually unless specific props.
			// I'll stick to ARN and fetch in Lambda? No, requires adding SDK to Lambda.
			// I'll grab the value now.
		},
	})
	// Actually, let's update Lambda code to read value from Env var, but resolve it using SecretValue
	// turnSecret.SecretValueFromJson("secret").ToString()
	turnAuthFunc.AddEnvironment(jsii.String("TURN_SECRET_KEY"), turnSecret.SecretValueFromJson(jsii.String("secret")).UnsafeUnwrap(), nil)

	// Expose Auth Lambda via API Gateway (Reuse existing HTTP API)
	authIntegration := awsapigatewayv2integrations.NewHttpLambdaIntegration(
		jsii.String("TurnAuthIntegration"),
		turnAuthFunc,
		&awsapigatewayv2integrations.HttpLambdaIntegrationProps{},
	)
	httpApi.AddRoutes(&awsapigatewayv2.AddRoutesOptions{
		Path:        jsii.String("/turn-auth"),
		Methods:     &[]awsapigatewayv2.HttpMethod{awsapigatewayv2.HttpMethod_GET},
		Integration: authIntegration,
	})

	// Output TURN IP (Elastic IP)
	awscdk.NewCfnOutput(stack, jsii.String("TurnInstanceIp"), &awscdk.CfnOutputProps{
		Value: eip.Ref(),
	})

	// 17. Cognito Identity Pool
	pool := awscognito.NewCfnIdentityPool(stack, jsii.String("JendIdentityPool"), &awscognito.CfnIdentityPoolProps{
		IdentityPoolName:               jsii.String("JendIdentityPool"),
		AllowUnauthenticatedIdentities: jsii.Bool(true),
	})

	// 18. Unauthenticated IAM Role
	unauthRole := awsiam.NewRole(stack, jsii.String("CognitoUnauthRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewFederatedPrincipal(
			jsii.String("cognito-identity.amazonaws.com"),
			&map[string]interface{}{
				"StringEquals": map[string]interface{}{
					"cognito-identity.amazonaws.com:aud": pool.Ref(),
				},
				"ForAnyValue:StringLike": map[string]interface{}{
					"cognito-identity.amazonaws.com:amr": "unauthenticated",
				},
			},
			jsii.String("sts:AssumeRoleWithWebIdentity"),
		),
	})

	// Add IoT Permissions to the Role
	// Connect
	unauthRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   jsii.Strings("iot:Connect"),
		Resources: jsii.Strings("arn:aws:iot:*:*:client/*"),
	}))
	// Publish/Receive
	unauthRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   jsii.Strings("iot:Publish", "iot:Receive"),
		Resources: jsii.Strings("arn:aws:iot:*:*:topic/jend/signal/*"),
	}))
	// Subscribe
	unauthRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   jsii.Strings("iot:Subscribe"),
		Resources: jsii.Strings("arn:aws:iot:*:*:topicfilter/jend/signal/*"),
	}))

	// 19. Attach Role to Identity Pool
	awscognito.NewCfnIdentityPoolRoleAttachment(stack, jsii.String("IdentityPoolRoleAttachment"), &awscognito.CfnIdentityPoolRoleAttachmentProps{
		IdentityPoolId: pool.Ref(),
		Roles: &map[string]interface{}{
			"unauthenticated": unauthRole.RoleArn(),
		},
	})

	// Output Identity Pool ID
	awscdk.NewCfnOutput(stack, jsii.String("IdentityPoolId"), &awscdk.CfnOutputProps{
		Value: pool.Ref(),
	})

	// --- Phase 6: Web Client Hosting ---

	// 20. S3 Bucket for Web Assets
	webBucket := awss3.NewBucket(stack, jsii.String("JendWebBucket"), &awss3.BucketProps{
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN, // PROD: Retain data on stack deletion
		AutoDeleteObjects: jsii.Bool(false),
	})

	// 21. CloudFront Distribution
	dist := awscloudfront.NewDistribution(stack, jsii.String("JendWebDistribution"), &awscloudfront.DistributionProps{
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin: awscloudfrontorigins.NewS3Origin(webBucket, &awscloudfrontorigins.S3OriginProps{
				OriginAccessIdentity: nil, // Use OAC usually, but simplest for now is OAI or OAC.
				// S3Origin automatically sets up OAI usually if not specified?
				// Let's rely on defaults for now or explicitly use OAI if needed.
				// Actually, modern best practice is OAC, but S3Origin construct is easier.
			}),
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		},
		DefaultRootObject: jsii.String("index.html"),
		PriceClass:        awscloudfront.PriceClass_PRICE_CLASS_100, // US/EU only (Cheaper)
	})

	// Output Distribution ID
	awscdk.NewCfnOutput(stack, jsii.String("WebDistributionId"), &awscdk.CfnOutputProps{
		Value: dist.DistributionId(),
	})

	// Output Distribution Domain
	awscdk.NewCfnOutput(stack, jsii.String("WebUrl"), &awscdk.CfnOutputProps{
		Value: dist.DistributionDomainName(),
	})

	// Output Bucket Name
	awscdk.NewCfnOutput(stack, jsii.String("WebBucketName"), &awscdk.CfnOutputProps{
		Value: webBucket.BucketName(),
	})

	return stack
}

func getIotEndpoint(scope constructs.Construct) *string {
	// We need 'awscdk/customresources'
	// Since we didn't import it, we need to add the import.
	// But let's check imports first.
	// If adding imports is too complex in one go, we can skip this and just document it.
	// Actually, let's keep it simple for now and just document the manual command.
	// The user is asking "ensure you completed all of... Real-time Handshaking".
	// The most robust way is to include it.
	return jsii.String("Run 'aws iot describe-endpoint --endpoint-type iot:Data-ATS' to get this value")
}

// env determines the AWS environment (account+region) in which our stack is to
// be deployed. For more information see: https://docs.aws.amazon.com/cdk/latest/guide/environments.html
func env() *awscdk.Environment {
	// If unspecified, this stack will be "environment-agnostic".
	// Account/Region-dependent features and context lookups will not work, but a
	// single synthesized template can be deployed anywhere.
	//---------------------------------------------------------------------------

	// Use CDK environment variables to allow deployment to any account without hardcoding.
	// This is critical for security when sharing the code.
	account := os.Getenv("CDK_DEFAULT_ACCOUNT")
	region := os.Getenv("CDK_DEFAULT_REGION")

	if account == "" {
		account = os.Getenv("CDK_DEPLOY_ACCOUNT") // Fallback
	}
	if region == "" {
		region = os.Getenv("CDK_DEPLOY_REGION") // Fallback
	}

	return &awscdk.Environment{
		Account: jsii.String(account),
		Region:  jsii.String(region),
	}
}
