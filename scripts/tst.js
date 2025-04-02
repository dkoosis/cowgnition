const http = require('http');

function testMcpServer() {
  console.log('Testing MCP server at http://localhost:8080/mcp/initialize');

  // Initialize request
  const initRequest = {
    protocolVersion: "2.0",
    clientInfo: {
      name: "test-client",
      version: "1.0.0"
    },
    capabilities: {}
  };

  const requestData = JSON.stringify(initRequest);
  console.log('Sending initialize request:', requestData);

  // Options for the HTTP request
  const options = {
    hostname: 'localhost',
    port: 8080,
    path: '/mcp/initialize',
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Content-Length': Buffer.byteLength(requestData)
    }
  };

  // Make the HTTP request
  const req = http.request(options, (res) => {
    console.log('Response status:', res.statusCode);

    let responseData = '';
    res.on('data', (chunk) => {
      responseData += chunk;
    });

    res.on('end', () => {
      try {
        const parsedData = JSON.parse(responseData);
        console.log('Response data:', JSON.stringify(parsedData, null, 2));

        // If initialize was successful, try listing resources
        if (res.statusCode === 200) {
          console.log('\nTesting resource listing...');
          testListResources();
        }
      } catch (e) {
        console.error('Error parsing response:', e);
        console.log('Raw response:', responseData);
      }
    });
  });

  req.on('error', (e) => {
    console.error('Error making request:', e);
  });

  // Write data to request body
  req.write(requestData);
  req.end();
}

function testListResources() {
  const options = {
    hostname: 'localhost',
    port: 8080,
    path: '/mcp/list_resources',
    method: 'GET',
    headers: {
      'Content-Type': 'application/json'
    }
  };

  const req = http.request(options, (res) => {
    console.log('Resource response status:', res.statusCode);

    let responseData = '';
    res.on('data', (chunk) => {
      responseData += chunk;
    });

    res.on('end', () => {
      try {
        const parsedData = JSON.parse(responseData);
        console.log('Resource data:', JSON.stringify(parsedData, null, 2));
      } catch (e) {
        console.error('Error parsing resource response:', e);
        console.log('Raw resource response:', responseData);
      }
    });
  });

  req.on('error', (e) => {
    console.error('Error making resource request:', e);
  });

  req.end();
}

testMcpServer();
