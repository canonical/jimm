#!/bin/bash

echo "Checking main network interface for suitability for MetalLB..."
echo

# Get default route interface & details for it
MAIN_INTERFACE_NAME=$(ip route | awk '/default/ {print $5; exit}')
MAIN_INTERFACE_DETAILS=$(ip -4 addr show dev "$MAIN_INTERFACE_NAME")

echo "Main interface name: $MAIN_INTERFACE_NAME"
echo "Main interface details: $MAIN_INTERFACE_DETAILS"
echo


# Extract flags from interface details
FLAGS=$(echo "$MAIN_INTERFACE_DETAILS" | grep -oP '(?<=<)[^>]+')

# Make sure it is UP (so enabled on system level), 
# LOWER_UP(link is active), global(accessible on this subnetmask) and not loopbackc 
if [[ "$FLAGS" == *UP* ]] && [[ "$FLAGS" == *LOWER_UP* ]] && [[ "$FLAGS" != *LOOPBACK* ]]; then
    echo "Interface is UP, link active, and not loopback"
else
    echo "Interface not suitable"
    exit 1
fi

# Extract inet line from interface details
INET_INFO=$(echo "$MAIN_INTERFACE_DETAILS" | awk '/inet /')
# Make sure it is global and has an IP address assigned
if [[ "$INET_INFO" == *global* ]] && [[ "$INET_INFO" == *inet* ]]; then
    echo "Interface has global scope and an IP address assigned"
else
    echo "Interface does not have global scope or an IP address assigned"
    exit 1
fi

# Get the IP for this interface (this is us)
IP="$(echo "$MAIN_INTERFACE_DETAILS" | awk '/inet/ {print $2}' | cut -d/ -f1)"
# Now get cidr for subnet (make sure it's at least 24 for our simple testing) and we can compute
# a safe range for metallb ARP
CIDR=$(echo "$MAIN_INTERFACE_DETAILS" | awk '/inet/ {print $2}' | cut -d/ -f2)
if [ "$CIDR" -gt 24 ]; then
    echo "Subnet too small for MetalLB"
    exit 1
else
    echo "Subnet: /$CIDR"
    echo "Subnet is suitable for MetalLB"
fi

echo "Machine IP is: $IP"

# Increment the last number by 1 for the start of the MetalLB range
# And give us a range of 5 ips for the lb to use in the arp table
# TODO: This will break if the initial machine IP is at the end of a subnet
# e.g., x.x.x.254/24 or x.x.x.255/24
# so it should loop over the lact octet and find a suitable range.
VALID_IPS=()
for i in $(seq 1 5); do
    METALLB_IP=$(echo "$IP" | awk -F. -v i="$i" '{print $1 "." $2 "." $3 "." $4+i}')
    echo "Testing IP: $METALLB_IP"

    # Extract the last octet of the IP address so we can make sure:
    # - we don't use .0, .1 or .255
    # - ping fails, and after pinging it isn't in ARP cache (so not in use)
    # and if all is ok, we'll add it to the set of ips for metallb to use.
    # this is a bit janky but I'm not sure how else to do this.
    LAST_OCTET=$(echo "$METALLB_IP" | awk -F. '{print $4}')

    # Check if the last octet is valid for MetalLB (0,1,255 are not)
    if [[ "$LAST_OCTET" -le 1 || "$LAST_OCTET" -ge 255 ]]; then
        echo "Last octet is invalid, skipping"
        continue
    fi

    if ping -c1 -W1 "$METALLB_IP" >/dev/null 2>&1; then
        echo "IP $METALLB_IP responds to ping, skipping"
        continue
    fi

    # Check ip neigh
    STATUS=$(ip neigh show "$METALLB_IP" | awk '{print $5}')

    # TODO: check stale is the right status to disagnose as in use
    if [[ "$STATUS" == "REACHABLE" || "$STATUS" == "STALE" ]]; then
        echo "$METALLB_IP is in use, skipping"
        continue
    fi

    VALID_IPS+=("$METALLB_IP")
done

if [ "${#VALID_IPS[@]}" -lt 5 ]; then
    echo "Not enough valid IPs found for MetalLB"
    exit 1
fi
echo "Valid IPs for MetalLB: ${VALID_IPS[*]}"
METALLB_RANGE="${VALID_IPS[0]}-${VALID_IPS[-1]}"
echo "Enabling metallb with range: $METALLB_RANGE" 

sudo microk8s enable metallb:"$METALLB_RANGE"
