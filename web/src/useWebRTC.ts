import { useState, useRef, useCallback } from 'react';
import type { SignalingMessage } from './useSignaling';
import { useSignaling, API_ENDPOINT } from './useSignaling';

const CHUNK_SIZE = 16 * 1024; // 16 KB chunks for stable WebRTC sending

export interface TransferState {
    status: 'idle' | 'connecting' | 'transferring' | 'done' | 'error';
    progress: number;
    fileName?: string;
    fileSize?: number;
}

export function useWebRTC(transferCode: string, isSender: boolean) {
    const { isConnected, error: sigError, connect, sendSignal, clientId } = useSignaling(transferCode);

    const peerConnection = useRef<RTCPeerConnection | null>(null);
    const dataChannel = useRef<RTCDataChannel | null>(null);
    const receiveBuffer = useRef<Uint8Array[]>([]);
    const receivedSize = useRef(0);
    const expectedSize = useRef(0);
    const expectedFileName = useRef('download');

    const [state, setState] = useState<TransferState>({ status: 'idle', progress: 0 });

    const initWebRTC = useCallback(async () => {
        setState({ status: 'connecting', progress: 0 });

        let iceServers: RTCIceServer[] = [{ urls: 'stun:stun.l.google.com:19302' }];

        try {
            const resp = await fetch(`${API_ENDPOINT}/turn-auth`);
            if (resp.ok) {
                const creds = await resp.json();
                if (creds.uris && creds.username && creds.password) {
                    iceServers = [
                        ...iceServers,
                        ...creds.uris.map((uri: string) => ({
                            urls: uri,
                            username: creds.username,
                            credential: creds.password
                        }))
                    ];
                    console.log('Successfully loaded Cloud TURN credentials for restricted networks');
                }
            } else {
                console.warn('Failed to fetch TURN credentials, status:', resp.status);
            }
        } catch (e) {
            console.warn('Could not fetch TURN credentials, falling back to STUN-only', e);
        }

        // Use Google's free public STUN servers + AWS TURN for NAT hole-punching
        const pc = new RTCPeerConnection({
            iceServers
        });

        peerConnection.current = pc;

        pc.onicecandidate = (event) => {
            if (event.candidate) {
                sendSignal({
                    type: 'candidate',
                    sender_id: clientId,
                    data: event.candidate
                });
            }
        };

        if (isSender) {
            // Sender creates the DataChannel
            const dc = pc.createDataChannel('jend-transfer');
            setupDataChannel(dc);

            // Create and send offer
            pc.createOffer().then(offer => {
                pc.setLocalDescription(offer);
                sendSignal({ type: 'offer', sender_id: clientId, data: offer });
            });
        } else {
            // Receiver waits for DataChannel
            pc.ondatachannel = (event) => {
                setupDataChannel(event.channel);
            };
        }
    }, [isSender, sendSignal, clientId]);

    const setupDataChannel = (dc: RTCDataChannel) => {
        dc.binaryType = 'arraybuffer';
        dataChannel.current = dc;

        dc.onopen = () => {
            console.log('Data channel open!');
            if (!isSender) {
                setState({ status: 'transferring', progress: 0 });
            }
        };

        dc.onmessage = (event) => {
            if (typeof event.data === 'string') {
                // Metadata message
                const meta = JSON.parse(event.data);
                if (meta.type === 'metadata') {
                    expectedSize.current = meta.size;
                    expectedFileName.current = meta.name;
                    setState(s => ({ ...s, fileName: meta.name, fileSize: meta.size }));
                }
            } else {
                // Binary chunk
                const chunk = new Uint8Array(event.data);
                receiveBuffer.current.push(chunk);
                receivedSize.current += chunk.byteLength;

                setState(s => ({
                    ...s,
                    progress: Math.floor((receivedSize.current / expectedSize.current) * 100)
                }));

                if (receivedSize.current >= expectedSize.current) {
                    downloadFile();
                }
            }
        };
    };

    const handleSignalingMessage = useCallback(async (msg: SignalingMessage) => {
        if (msg.sender_id === clientId) return; // Ignore our own echoes
        const pc = peerConnection.current;
        if (!pc) return;

        if (msg.type === 'offer') {
            await pc.setRemoteDescription(new RTCSessionDescription(msg.data));
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            sendSignal({ type: 'answer', sender_id: clientId, data: answer });
        } else if (msg.type === 'answer') {
            await pc.setRemoteDescription(new RTCSessionDescription(msg.data));
        } else if (msg.type === 'candidate') {
            await pc.addIceCandidate(new RTCIceCandidate(msg.data));
        }
    }, [clientId, sendSignal]);

    const startTransfer = useCallback((file: File) => {
        if (!dataChannel.current || dataChannel.current.readyState !== 'open') return;

        setState({ status: 'transferring', progress: 0, fileName: file.name, fileSize: file.size });

        // First, send the metadata so the receiver knows what is coming
        dataChannel.current.send(JSON.stringify({
            type: 'metadata',
            name: file.name,
            size: file.size
        }));

        // Start streaming the file in chunks
        let offset = 0;
        const reader = new FileReader();

        const readSlice = (o: number) => {
            const slice = file.slice(o, o + CHUNK_SIZE);
            reader.readAsArrayBuffer(slice);
        };

        reader.onload = (e) => {
            if (e.target?.result && dataChannel.current?.readyState === 'open') {
                // Warning: React buffers can fill up. Real production code uses dc.bufferedAmount limits here.
                dataChannel.current.send(e.target.result as ArrayBuffer);
                offset += CHUNK_SIZE;

                setState(s => ({ ...s, progress: Math.floor((offset / file.size) * 100) }));

                if (offset < file.size) {
                    readSlice(offset);
                } else {
                    setState(s => ({ ...s, status: 'done', progress: 100 }));
                }
            }
        };

        readSlice(offset);

    }, []);

    const downloadFile = () => {
        const blob = new Blob(receiveBuffer.current as BlobPart[]);
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = expectedFileName.current;
        a.click();
        URL.revokeObjectURL(url);

        setState(s => ({ ...s, status: 'done' }));
        receiveBuffer.current = [];
    };

    return { state, sigError, initWebRTC, startTransfer, handleSignalingMessage, isSigConnected: isConnected, connectSignaling: connect };
}
