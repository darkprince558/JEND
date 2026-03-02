# JEND Deployment Guide

While JEND works purely peer-to-peer on local networks out of the box, its true power comes from its cloud infrastructure, which enables NAT traversal (hole punching) across strict firewalls and a seamless web-based receiver for users without the CLI installed.

This document explains how to deploy the supporting AWS infrastructure and the React web frontend.

## 1. AWS Infrastructure (CDK)

JEND uses the AWS Cloud Development Kit (CDK) to provision its backend services. The backend handles:

- **Registry & Discovery:** DynamoDB + Lambda for matching sender/receiver connection offers.
- **Relay (TURN/STUN):** An EC2 instance running `coturn` to relay WebRTC/QUIC traffic when direct P2P is blocked (e.g., symmetric NATs, enterprise firewalls).
- **Authentication:** Amazon Cognito for identity management.
- **Cloud Storage (S3 Mode):** An S3 bucket for temporary, asynchronous file transfers (`--s3` flag).

### Deployment Steps

1. Configure your AWS credentials (`aws configure`).
2. Navigate to the `infra` directory:

   ```bash
   cd infra
   ```

3. Install dependencies and bootstrap the CDK (if this is your first time using CDK in this account/region):

   ```bash
   npm install
   npx cdk bootstrap
   ```

4. Deploy the stack:

   ```bash
   npx cdk deploy
   ```

   This will output the created API Gateway endpoints, Cognito Identity Pool ID, and S3 Bucket name.

### Updating the App Settings

After deployment, you need to update the default identifiers in the JEND Go code if you want to use your custom deployment.
Update the constants in `internal/config/defaults.go`:

- `DefaultIdentityPoolID`
- `DefaultAPIEndpoint`
- `DefaultS3Bucket`

## 2. Web Frontend (React)

JEND includes a React web application (`web/` directory) that acts as a secure, browser-based receiver. This allows senders to share a QR code in their terminal, which the receiver can scan to download the file directly in their browser without installing JEND.

### Development

1. Navigate to the `web` directory:

   ```bash
   cd web
   ```

2. Install Node.js dependencies:

   ```bash
   npm install --legacy-peer-deps
   ```

3. Run the development server:

   ```bash
   npm run dev
   ```

### Building for Production

To build the optimized static bundle:

```bash
npm run build
```

The output will be generated in `web/dist`. You can host these static files on any specialized web host (e.g., Vercel, Netlify, AWS S3 + CloudFront, GitHub Pages).

### Configuration

Ensure the web app points to your deployed AWS infrastructure by updating the `API_ENDPOINT` and `IDENTITY_POOL_ID` constants in `web/src/useSignaling.ts`.
