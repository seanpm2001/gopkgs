// vi: sw=4 ts=4:

/*
------------------------------------------------------------------------------------------------
	Mnemonic:	ostack_net
	Abstract:	Functions to support getting information from the network component (o.nhost)
				of openstack. 

	Date:		03 February 2014 (hbdkrd)
	Author:		E. Scott Daniels

	Related:	Doc for agent api request is (of course) not with all of the other openstack
				networking doc, it's here:
					http://docs.openstack.org/api/openstack-network/2.0/content/List_Agents.html

				Other api doc:
					http://developer.openstack.org/api-ref-networking-v2.html

	Mods:		24 July 2014 - bloody icehouse has no backward compat inasmuch as the complete
					list of gateways cannot be fetched on one call. Users will need to 
					make a call per project, and we need to provde the support to update
					an existing map. 
				13 Aug 2014 - Added error checking centered round missing urls. 
					Moved deprecated code out and added a function which (through a hackish
					request) generates a list of physical hosts with network services running on them.
				30 Aug 2014 - Added more description to error message (object to string output).
				30 Sep 2014 - Looks like bloody openstack is returning host names which are of
					the form "host": "c1r2:1ed04397-35fb-51ca-a932-29d8e09be240". Why can't it
					drop the bloody uuname and make it a separate field. We now assume that 
					colon is not a leagal character in a host name and will split and drop
					anything after the first colon.
				14 Oct 2014 - Changed list_net_hosts to list only the OVS hosts.
				04 Oct 2014 - Changed list_net_hosts to look for the agent string "quantum-openvswitch-agent"
					to be compatable with grizzley (bloody openstack renaming mid-stream).
				04 Dec 2014 - Now reports network host only if service shows alive.
------------------------------------------------------------------------------------------------
*/

package ostack

import (
	//"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	//"io/ioutil"
	//"net/http"
	//"net/url"
	"os"
	"strings"
	//"time"

	//"forge.research.att.com/gopkgs/jsontools"
)

/*
	Generate a string containing a space separated list of physical host names which 
	are associated with the particular type of agent(s) that are passed in. 

	Udup_list is a map of host names that have already been encountered (dups) and should be 
	ignored; it can be nil.  The dup map generated is returned. 

*/
func (o *Ostack) List_net_hosts( udup_list map[string]bool, limit2neutron bool ) ( hlist *string, dup_map map[string]bool, err error ) {
	var (
		rdata generic_response		// stuff back from openstack
		jdata	[]byte				// raw json response data
	)

	empty_str := ""
	hlist = &empty_str
	dup_map = udup_list				// ensure it goes back even on error

	if o == nil {
		err = fmt.Errorf( "net_netinfo: openstack creds were nil" )
		return
	}

	err = o.Validate_auth()						// reauthorise if needed
	if err != nil {
		return
	}

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query in %s", o.To_str() )
		return
	}

	if dup_map == nil {
		dup_map = make( map[string]bool, 24 )
	} 

	jdata = nil
	body := bytes.NewBufferString( "" )

	url := fmt.Sprintf( "%s/v2.0/agents", *o.nhost )				// nhost is the host where the network service is running
	dump_url( "Mk_net_phost", 10, url )
	jdata, _, err = o.Send_req( "GET",  &url, body )

	if err != nil {
		return
	}

	err = json.Unmarshal( jdata, &rdata )			// unpack the json into jif(net_data)
	if err != nil {
		dump_json( "mk_net_phost", 30, jdata )
		return
	} else {
		dump_json( "mk_net_phost", 10, jdata )
	}

	wstr := ""
	sep := ""
	for i := range rdata.Agents {
		if (limit2neutron == false ||
			*rdata.Agents[i].Binary == "neutron-openvswitch-agent"  ||
			*rdata.Agents[i].Binary == "quantum-openvswitch-agent") &&
			rdata.Agents[i].Alive {									// list only if service is alive (assume host is also up)

			tokens := strings.SplitN( *rdata.Agents[i].Host, ".", 2 )	// ostack isn't consistent, these might come back fully qualified with domain; strip
			tokens = strings.SplitN( tokens[0], ":", 2 )				// and it sometimes adds :uuid to the name so trash that too
			
			if ! dup_map[tokens[0]] {
				wstr += sep + tokens[0]
				sep = " "
				dup_map[tokens[0]]  = true
			}
		}
	}

	hlist = &wstr
	return
}

/*
	Generate a map that is keyed by the network name with each entrying beign a three tuple, space
	separated, string of: phyiscial net, type (gre,vlan,etc), and segment id.
*/
func (o *Ostack) Mk_netinfo_map( ) ( nmap map[string]*string, err error ) {
	var (
		net_list generic_response	// top level data mapped from ostack json
		jdata	[]byte				// raw json response data
	)

	nmap = nil
	if o == nil {
		err = fmt.Errorf( "net_netinfo: openstack creds were nil" )
		return
	}

	err = o.Validate_auth()						// reauthorise if needed
	if err != nil {
		return
	}

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query %s", o.To_str()  )
		return
	}

	jdata = nil
	body := bytes.NewBufferString( "" )

	url := fmt.Sprintf( "%s/v2.0/networks", *o.nhost )				// nhost is the host where the network service is running
	dump_url( "Mk_netinfo_map", 10, url )
	jdata, _, err = o.Send_req( "GET",  &url, body )

	if err != nil {
		return
	}

	err = json.Unmarshal( jdata, &net_list )			// unpack the json into jif(net_data)
	if err != nil {
		fmt.Fprintf( os.Stderr, "ostack/net_netinfo: unable to unpack json: %s\n", err )		//TESTING
		fmt.Fprintf( os.Stderr, "offending_json=%s\n", jdata )
		return
	}

	nmap = make( map[string]*string, 101 )				// size is a hint, not a limit
	for _, n := range net_list.Networks {
		dup_str :=fmt.Sprintf( "%s %s %s %d", n.Id, n.Phys_net, n.Phys_type, n.Phys_seg_id )
		nmap[n.Name] = &dup_str
	}

	return
}

/*
 	Generate a map from [tenant]/ip to mac or the reverse.
	User map is updated if not nil, otherwise a new map is created.
*/
func (o *Ostack) gwmac2xip(  umap map[string]*string, usr_jdata []byte, inc_tenant bool, reverse bool ) ( m map[string]*string, err error ) {
	var (
		ports 	generic_response			// unpacked json from response
		addr	string				// ip address
		jdata	[]byte				// raw json response data
	)

	m = umap
	if o == nil {
		err = fmt.Errorf( "net_gwmac2xip: openstack creds were nil" )
		return
	}

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query %s", o.To_str()  )
		return
	}


	if usr_jdata == nil {
		err = o.Validate_auth()						// reauthorise if needed
		if err != nil {
			return
		}

		jdata = nil
		body := bytes.NewBufferString( "" )

		url := fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface&tenant_id=%s", *o.nhost, *o.project_id )				// lists just the gateways
		jdata, _, err = o.Send_req( "GET",  &url, body )

		if err != nil {
			return
		}
	} else {
		jdata = usr_jdata
	}

	err = json.Unmarshal( jdata, &ports )			// unpack the json into jif
	if err != nil {
		fmt.Fprintf( os.Stderr, "ostack/gwmac2xip: unable to unpack json: %s\n", err )		//TESTING
		fmt.Fprintf( os.Stderr, "offending_json=%s\n", jdata )
		return
	}

	if m == nil {								// no user map supplied, then create a new one
		m = make( map[string]*string )
	}

	for j := range ports.Ports {
		if inc_tenant {
			addr = ports.Ports[j].Tenant_id + "/" + ports.Ports[j].Fixed_ips[0].Ip_address
		} else {
			addr = ports.Ports[j].Fixed_ips[0].Ip_address
		}

		dup_addr := addr;						// MUST duplicate them 
		dup_mac := ports.Ports[j].Mac_address

		if reverse {
			m[dup_addr] = &dup_mac
		} else {
			m[dup_mac] = &dup_addr
		}
	}

	return
}

/*
	Generates gateway [tenant/]ip to mac and mac to [tenant/]ip maps. Needs only one call to openstack 
	to generate both maps. 

	The u* maps are updated if supplied. If nil is passed, a new map is created.
	If use_project is true, then the request is made using the project_id, otherwise the 
	project_id is not submitted. In versions before icehouse, submitting without the project
	ID,  with an admin user ID, resulted in a complete list of gateways. With icehouse, it 
	seems that we must request for each project.
*/
func (o *Ostack) Mk_gwmaps( umac2ip map[string]*string, uip2mac map[string]*string, inc_tenant bool, use_project bool ) ( mac2ip map[string]*string, ip2mac map[string]*string, err error ) {
	var (
		jdata []byte
		url	string
	)

	ip2mac = uip2mac							// ensure we return the user maps on error
	mac2ip = umac2ip

	err = o.Validate_auth()						// reauthorise if needed
	if err != nil {
		return
	}

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query %s", o.To_str() )
		return
	}

	jdata = nil
	body := bytes.NewBufferString( "" )

	//url := fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface", *o.nhost )				// lists just the gateways
	if use_project {
		url = fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface&tenant_id=%s", *o.nhost, *o.project_id )				// bloody icehouse
	} else {
		url = fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface", *o.nhost )		// before icehouse all are returned on single generic call
	}
	dump_url( "Mk_gwmaps", 10, url )
	jdata, _, err = o.Send_req( "GET",  &url, body )
	dump_json( "Mk_gwmaps", 10, jdata )

	if err != nil {
		return
	}

	ip2mac, err = o.gwmac2xip( ip2mac, jdata, inc_tenant, true )
	if err != nil {
		return
	}
	mac2ip, err = o.gwmac2xip( mac2ip, jdata, inc_tenant, false )

	return
}

/*
 	Creates a list of IP addresses that are gateways. 	
*/
func (o *Ostack) Mk_gwlist( ) ( gwlist []string, err error ) {
	var (
		ports 	generic_response			// unpacked json from response
		jdata	[]byte				// raw json response data
		url		string
	)

	gwlist = nil

	if o == nil {
		err = fmt.Errorf( "net_subnets: openstack creds were nil" )
		return
	}

	err = o.Validate_auth()						// reauthorise if needed
	if err != nil {
		return
	}

	jdata = nil
	body := bytes.NewBufferString( "" )

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query %s", o.To_str() )
		return
	}

	//url := fmt.Sprintf( "%s/v2.0/subnets", *o.nhost )				// nhost is the host where the network service is running
	if o.project != nil {
		url = fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface&tenant_id=%s", *o.nhost, *o.project_id )				// lists just the gateways
	} else {
		url = fmt.Sprintf( "%s/v2.0/ports?device_owner=network:router_interface", *o.nhost )		// before icehouse all are returned on single generic call, so nil project is acceptable
	}
	jdata, _, err = o.Send_req( "GET",  &url, body )

	if err != nil {
		return
	}

	err = json.Unmarshal( jdata, &ports )			// unpack the json into jif
	if err != nil {
		fmt.Fprintf( os.Stderr, "ostack/net_subnet: unable to unpack json: %s\n", err )		//TESTING
		fmt.Fprintf( os.Stderr, "offending_json=%s\n", jdata )
		return
	}

	gwlist = make( []string, len( ports.Ports ) )
	i := 0
	for j := range ports.Ports {
		gwlist[i] = fmt.Sprintf( "%s %s", ports.Ports[j].Mac_address, ports.Ports[j].Fixed_ips[0].Ip_address )
		i++
	}

	return
}


func (o *Ostack) Dump_json( uurl string ) ( err error ) {
	var (
		jdata	[]byte				// raw json response data
	)

	if o == nil {
		err = fmt.Errorf( "net_subnets: openstack creds were nil" )
		return
	}

	err = o.Validate_auth()						// reauthorise if needed
	if err != nil {
		return
	}

	if o.nhost == nil || *o.nhost == "" {
		err = fmt.Errorf( "no network host url to query %s", o.To_str() )
		return
	}

	jdata = nil
	body := bytes.NewBufferString( "" )

	//url := fmt.Sprintf( "%s/v2.0/subnets", *o.nhost )				// nhost is the host where the network service is running
	//url := fmt.Sprintf( "%s/%s", *o.nhost, uurl )				// nhost is the host where the network service is running
	url := fmt.Sprintf( "%s/%s", *o.chost, uurl )				// nhost is the host where the network service is running
	jdata, _, err = o.Send_req( "GET",  &url, body )

	if err != nil {
		fmt.Fprintf( os.Stderr, "error: %s\n", err )
		return
	}

	fmt.Fprintf( os.Stderr, "json= %s\n", jdata )

	return
}

