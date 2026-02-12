# BACnet Controller

This BACnet controller implements a BACnet/IP device that exposes thermostat properties via standard BACnet services.

## Features

- **Who-Is/I-Am**: Responds to Who-Is requests with I-Am messages
- **ReadProperty**: Supports reading thermostat properties via ReadProperty requests
- **WriteProperty**: Supports writing thermostat properties via WriteProperty requests

## BACnet Object Mapping

The controller exposes the following BACnet objects:

### Analog Input (Object Type 0)
- **Instance 0**: Ambient Temperature (PresentValue property)

### Analog Value (Object Type 2)
- **Instance 1**: Temperature Setpoint (PresentValue property)
- **Instance 2**: Temperature Setpoint Minimum (PresentValue property)
- **Instance 3**: Temperature Setpoint Maximum (PresentValue property)

### Binary Input (Object Type 3)
- **Instance 4**: Enabled State (PresentValue property) - 0=disabled, 1=enabled

### Multi-state Value (Object Type 19)
- **Instance 5**: Mode (PresentValue property) - 1=heat, 2=cool, 3=fan, 4=auto
- **Instance 6**: Fan Speed (PresentValue property) - 1=auto, 2=low, 3=medium, 4=high

## Configuration

```yaml
controllers:
  bacnet:
    enabled: true
    addr: "0.0.0.0:47808"  # BACnet/IP port (default: 47808)
    device_instance: 123   # BACnet device instance number (0-4194303)
```

## Usage

The BACnet controller can be used with any BACnet client to:

1. **Discover the device**: Send a Who-Is request to discover the thermostat device
2. **Read properties**: Use ReadProperty requests to get current thermostat values
3. **Write properties**: Use WriteProperty requests to control the thermostat

## Example BACnet Client Usage

```go
// Discover the device
whoIs := &services.UnconfirmedWhoIs{/* ... */}
// Send Who-Is, receive I-Am response

// Read ambient temperature
readReq := &services.ConfirmedReadProperty{
    ObjectType: 0,    // Analog Input
    InstanceId: 0,    // Ambient Temperature
    PropertyId: 85,   // PresentValue
}
// Send ReadProperty, receive ComplexACK with temperature value

// Write setpoint
writeReq := &services.ConfirmedWriteProperty{
    ObjectType: 2,    // Analog Value
    InstanceId: 1,    // Temperature Setpoint
    PropertyId: 85,   // PresentValue
    Value: 22.5,      // New setpoint value
}
// Send WriteProperty, receive SimpleACK on success
```

## Error Handling

The controller responds with appropriate BACnet error responses for:
- Unknown objects or properties
- Invalid data types
- Write access denied (e.g., invalid temperature ranges)

## Testing

Run the BACnet controller tests:

```bash
go test ./internal/controllers/bacnet/...
```

The tests verify:
- Who-Is/I-Am discovery functionality
- ReadProperty requests for all supported properties
- WriteProperty requests for writable properties