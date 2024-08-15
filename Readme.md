# Peer2Peer Connector Overview

**Peer2Peer Connector** is a WebSocket server designed to establish and manage real-time, low-latency connections between clients. It is the core component for enabling seamless peer-to-peer interactions and room management, making it an ideal solution for applications that require real-time data exchange and collaboration.

## Key Features

### 1. Client Connection Management
- Peer2Peer Connector allows multiple clients to connect and communicate with each other in real-time. Upon connection, each client receives a unique identifier, which is used to manage and route messages between peers.
- This unique identifier ensures that messages are accurately directed, whether the communication is between two clients or within a group or room.

### 2. Room Management
- Peer2Peer Connector supports the creation and management of virtual rooms, where clients can dynamically join, leave, or be removed from rooms.
- Each room is identified by a unique room ID, allowing clients to join rooms based on their needs. This feature is ideal for applications like chat rooms, collaborative workspaces, or multiplayer games.

### 3. WebRTC Connection Establishment
- Peer2Peer Connector is specifically designed to facilitate the establishment of WebRTC (Web Real-Time Communication) connections between clients. WebRTC enables direct peer-to-peer communication, allowing for high-quality video, audio, and data sharing without intermediaries.
- The server acts as a signaling channel, enabling clients to exchange the necessary signaling data (such as SDP and ICE candidates) required to set up a WebRTC connection seamlessly.

### 4. Message Routing and Error Handling
- Peer2Peer Connector efficiently routes messages between clients, ensuring accurate and timely delivery of data. It supports various message types, including `connect`, `create_room`, `offer`, `answer`, `candidate`, and general `message`.
- In addition to message routing, Peer2Peer Connector offers robust error handling. If a client attempts to send a message to a non-existent peer or fails to provide the necessary data for a WebRTC connection, the server responds with clear error messages, guiding the client in resolving the issue.

## Supported Message Types

- **`Connect`**: Used to initiate a connection with another client. This message should include data such as the SDP and candidate information.
- **`Offer`**: Part of the WebRTC connection process, sent by a client to initiate a peer-to-peer connection.
- **`Answer`**: Sent in response to an `offer`, completing the WebRTC connection setup.
- **`Candidate`**: Contains ICE candidate information necessary for establishing the WebRTC connection.
- **`Message`**: General-purpose message type for sending data between connected clients.
- **`Create_Rom`**: Used to create a new room. The message should include the `room` and optionally the `name` of the room, inside `data` field.
- **`Join_Room`**: Used to join a room. The message should include the `room` inside `data` field.
- **`Leave_Room`**: Used to leave a room. The message should include the `room` inside `data` field.
- **`End_Room`**: Used to end a room. The message should include the `room` inside `data` field.

For full documentation [Visit here](http://peer2peerconnector.shankarammai.com.np "Visit here") 


## How to Contribute

We welcome contributions to the Peer2Peer Connector project! Here's how you can get involved:

1. **Fork the Repository**: Start by forking the repository and cloning it to your local machine.
2. **Create a New Branch**: Work on your changes in a separate branch. This makes it easier to manage different features or fixes.
3. **Write and Test Your Code**: Make sure your changes are well-tested. You can self-host the server if you want to test it locally.
4. **Submit a Pull Request**: Once your changes are ready, submit a pull request for review. Be sure to include a detailed description of what your changes do.

### Testing Your Changes

For testing, you can use the following link to interact with a live instance of the Peer2Peer Connector: [peer2peerconnector.shankarammai.com.np](https://peer2peerconnector.shankarammai.com.np).

If you'd like to self-host the server for testing:

1. Clone the repository.
2. Follow the setup instructions in the documentation.
3. Run the server locally and use WebRTC clients to connect and test the functionality.

We appreciate your contributions and look forward to collaborating with you!

